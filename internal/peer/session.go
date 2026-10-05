package peer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Session struct {
	config    *Config
	state     ServerState
	stateMu   sync.RWMutex
	conn      *tls.Conn
	ctx       context.Context
	cancel    context.CancelFunc
	nextID    atomic.Uint64
	pendingMu sync.Mutex
	pending   map[uint64]chan []byte
	writeMu   sync.Mutex
}

var ErrSessionActive = errors.New("session already active")
var ErrMarshalling = errors.New("error marshaling json")

func NewSession(cfg *Config) *Session {
	return &Session{
		config:  cfg,
		state:   StateIdle,
		pending: (make(map[uint64]chan []byte)),
	}
}

func (s *Session) Attach(parentCtx context.Context, conn *tls.Conn) error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if s.state == StateActive {
		conn.Close()
		return fmt.Errorf("starting session: %w", ErrSessionActive)
	}

	s.state = StateActive

	s.conn = conn

	ctx, cancel := context.WithCancel(parentCtx)

	s.ctx = ctx
	s.cancel = cancel

	go s.readLoop()
	go s.heartbeatLoop()

	return nil
}

func (s *Session) writeFrame(data []byte) error {
	frame := append(data, '\n')

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.conn.SetWriteDeadline(time.Now().Add(s.config.HandshakeTimeout))
	_, err := s.conn.Write(frame)

	return err
}

func (s *Session) SendRequest(ctx context.Context, method string, params []byte) ([]byte, error) {
	id := s.nextID.Add(1)

	respChan := make(chan []byte, 1)

	s.pendingMu.Lock()
	s.pending[id] = respChan
	s.pendingMu.Unlock()

	defer func() {
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
	}()

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling json: %w", ErrMarshalling)
	}

	err = s.writeFrame(payload)
	if err != nil {
		return nil, err
	}

	select {
	case resp := <-respChan:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.ctx.Done():
		return nil, errors.New("session closed")
	}

}

func (s *Session) readLoop() {
	scanner := bufio.NewScanner(s.conn)
	scanner.Buffer(make([]byte, 4096), s.config.MaxFrameSize)

	for {
		s.conn.SetReadDeadline(time.Now().Add(s.config.IdleTimeout))

		if !scanner.Scan() {
			break
		}

		rawBytes := scanner.Bytes()

		msgBytes := bytes.Clone(rawBytes)

		var envelope struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}

		if err := json.Unmarshal(msgBytes, &envelope); err != nil {
			continue
		}

		if envelope.Method == "" && envelope.ID > 0 {
			s.pendingMu.Lock()
			ch, ok := s.pending[envelope.ID]
			if ok {
				ch <- msgBytes
				(delete(s.pending, envelope.ID))
			}
			s.pendingMu.Unlock()
		}

		if envelope.Method != "" {
			if envelope.Method == "ping" && envelope.ID > 0 {
				res := Response{
					JSONRPC: "2.0",
					ID:      envelope.ID,
					Result:  json.RawMessage("\"pong\""),
				}
				payload, err := json.Marshal(res)
				if err != nil {
					continue
				}
				_ = s.writeFrame(payload)

			}
		}

	}
	s.Close()
}

func (s *Session) heartbeatLoop() {
	//periodic ping logic will go here
}

func (s *Session) Close() error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if s.state == StateIdle {
		return nil
	}

	s.state = StateIdle

	s.cancel()
	s.conn.Close()

	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()

	for id, respChan := range s.pending {
		close(respChan)
		delete(s.pending, id)
	}

	return nil
}
