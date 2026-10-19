package peer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Uhm-why/caduceus/internal/config"
	"github.com/Uhm-why/caduceus/internal/mailbox"
)

type Session struct {
	config     *config.Config
	state      ServerState
	stateMu    sync.RWMutex
	conn       *tls.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	nextID     atomic.Uint64
	pendingMu  sync.Mutex
	pending    map[uint64]chan []byte
	writeMu    sync.Mutex
	mailbox    mailbox.Mailbox
	localCaps  *Capabilities
	remoteCaps *Capabilities
	capsMu     sync.RWMutex
}

var ErrSessionActive = errors.New("session already active")
var ErrMarshalling = errors.New("error marshaling json")

func NewSession(cfg *config.Config, mb mailbox.Mailbox) *Session {
	return &Session{
		config:     cfg,
		state:      StateIdle,
		pending:    (make(map[uint64]chan []byte)),
		mailbox:    mb,
		localCaps:  DefaultCapabilities(),
		remoteCaps: nil,
	}
}

func (s *Session) State() ServerState {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	return s.state

}

func (s *Session) RemoteCapabilities() *Capabilities {
	s.capsMu.RLock()
	defer s.capsMu.RUnlock()

	return s.remoteCaps
}

func (s *Session) HasRemoteFeature(feature string) bool {
	s.capsMu.RLock()
	defer s.capsMu.RUnlock()

	if s.remoteCaps != nil && s.remoteCaps.HasFeature(feature) {
		return true
	}

	return false
}

func (s *Session) validatePeerCapabilities(remote *Capabilities) error {

	if remote == nil {
		return fmt.Errorf("remote capabilities nil")
	}

	if remote.Version == "" {
		return fmt.Errorf("remote version capabilities entry empty")
	}
	if s.localCaps.Version != remote.Version {
		return fmt.Errorf("major version mismatch between local and remote")
	}
	if !remote.HasFeature("messaging") {
		return fmt.Errorf("Remote missing messaging feature")
	}

	return nil
}

func (s *Session) PerformHandshake(ctx context.Context) error {
	s.stateMu.RLock()
	var currentState = s.state
	s.stateMu.RUnlock()
	if currentState != StateHandshake {
		return fmt.Errorf("session not in handshake state: %s", s.state)
	}

	localCapsBytes, err := json.Marshal(s.localCaps)
	if err != nil {
		return fmt.Errorf("marshalling local capabilities: %w", err)
	}

	peerResponse, err := s.SendRequest(ctx, "caduceus/handshake", localCapsBytes)
	if err != nil {
		s.Close()
		return fmt.Errorf("handshake request failed: %w", err)
	}

	var unmarshaledPeerRes Response

	if err := json.Unmarshal(peerResponse, &unmarshaledPeerRes); err != nil {
		s.Close()
		return fmt.Errorf("unmarshaling handshake response: %w", err)
	}

	if unmarshaledPeerRes.Error != nil {
		s.Close()
		return fmt.Errorf("peer rejected handshake: %s", unmarshaledPeerRes.Error.Message)
	}

	var remoteCaps Capabilities

	if err := json.Unmarshal(unmarshaledPeerRes.Result, &remoteCaps); err != nil {
		s.Close()
		return fmt.Errorf("unmarshaling remote capabilities: %w", err)
	}

	if err := s.validatePeerCapabilities(&remoteCaps); err != nil {
		s.Close()
		return fmt.Errorf("validating remote capabilities: %w", err)
	}

	s.capsMu.Lock()
	s.remoteCaps = &remoteCaps
	s.capsMu.Unlock()

	s.stateMu.Lock()
	s.state = StateActive
	s.stateMu.Unlock()

	go s.heartbeatLoop()
	return nil
}

func (s *Session) Attach(parentCtx context.Context, conn *tls.Conn) error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if s.state == StateActive || s.state == StateHandshake {
		conn.Close()
		return fmt.Errorf("starting session: %w", ErrSessionActive)
	}

	s.state = StateHandshake

	s.conn = conn

	ctx, cancel := context.WithCancel(parentCtx)

	s.ctx = ctx
	s.cancel = cancel

	go s.readLoop()
	//go s.heartbeatLoop()

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
	case resp, ok := <-respChan:
		if !ok {
			return nil, errors.New("session closed while awaiting response")
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.ctx.Done():
		return nil, errors.New("session closed")
	}

}

func (s *Session) DrainInbox() ([]mailbox.Message, error) {
	return s.mailbox.Drain()
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
			ID     uint64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params,omitempty"`
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
			if s.State() == StateHandshake {
				var caps Capabilities
				if envelope.Method == "caduceus/handshake" {
					if err := json.Unmarshal(envelope.Params, &caps); err != nil {
						continue
					}
					if err := s.validatePeerCapabilities(&caps); err != nil {
						log.Printf("error validating peer capabilities: %v", err)
						if envelope.ID > 0 {
							res := Response{
								JSONRPC: "2.0",
								ID:      envelope.ID,
								Error: &RPCError{
									Code:    -32602,
									Message: err.Error(),
								},
							}
							if payload, marshalErr := json.Marshal(res); marshalErr == nil {
								_ = s.writeFrame(payload)
							}
						}

						break

					}

					s.capsMu.Lock()
					s.remoteCaps = &caps
					s.capsMu.Unlock()

					capsBytes, err := json.Marshal(s.localCaps)
					if err != nil {
						continue
					}

					res := Response{
						JSONRPC: "2.0",
						ID:      envelope.ID,
						Result:  capsBytes,
					}

					payload, err := json.Marshal(res)
					if err != nil {
						continue
					}

					s.stateMu.Lock()
					s.state = StateActive
					s.stateMu.Unlock()

					_ = s.writeFrame(payload)

					go s.heartbeatLoop()

				} else {
					if envelope.ID > 0 {
						res := Response{
							JSONRPC: "2.0",
							ID:      envelope.ID,
							Error: &RPCError{
								Code:    -32602,
								Message: "handshake required",
							},
						}

						payload, err := json.Marshal(res)
						if err != nil {
							continue
						}
						s.writeFrame(payload)
					}
				}
				continue
			}

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

			} else if envelope.Method == "agent/message" && envelope.ID > 0 {
				var inbound struct {
					CorrelationID string `json:"correlation_id"`
					Kind          string `json:"kind"`
					Message       string `json:"message"`
				}
				_ = json.Unmarshal(envelope.Params, &inbound)
				if inbound.Kind == "" {
					inbound.Kind = "message"
				}
				sender := s.conn.RemoteAddr().String()
				err := s.mailbox.Enqueue(mailbox.Message{
					CorrelationID: inbound.CorrelationID,
					Kind:          inbound.Kind,
					Timestamp:     time.Now().UTC(),
					Sender:        sender,
					Content:       inbound.Message,
				})
				if err != nil {
					log.Printf("error enqueuing inbound message: %v", err)
				}
				res := Response{
					JSONRPC: "2.0",
					ID:      envelope.ID,
					Result:  json.RawMessage("\"peer received: message delivered successfully over mTLS 1.3\""),
				}
				outPayload, err := json.Marshal(res)
				if err != nil {
					continue
				}
				_ = s.writeFrame(outPayload)
			}
		}

	}
	s.Close()
}

func (s *Session) heartbeatLoop() {
	ticker := time.NewTicker(s.config.KeepAlive)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			_, err := s.SendRequest(s.ctx, "ping", nil)
			if err != nil {
				s.Close()
				return
			}
		}
	}
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
