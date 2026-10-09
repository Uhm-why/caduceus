package peer

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/Uhm-why/caduceus/internal/config"
)

type Listener struct {
	config   *config.Config
	listener net.Listener
}

func NewListener(cfg *config.Config) *Listener {
	return &Listener{
		config: cfg,
	}
}

func (l *Listener) Start(ctx context.Context) (<-chan *tls.Conn, <-chan error, error) {
	tlsCfg, err := TLSServerConfig(l.config)
	if err != nil {
		return nil, nil, err
	}

	tcpListener, err := net.Listen("tcp", l.config.ListenAddress)
	if err != nil {
		return nil, nil, err
	}

	l.listener = tls.NewListener(tcpListener, tlsCfg)

	connChan := make(chan *tls.Conn)
	errChan := make(chan error, 1)

	go func() {
		<-ctx.Done()
		l.listener.Close()
	}()

	go l.acceptLoop(ctx, connChan, errChan)

	return connChan, errChan, nil
}

func (l *Listener) acceptLoop(ctx context.Context, connChan chan<- *tls.Conn, errChan chan<- error) {
	defer close(connChan)
	for {
		conn, err := l.listener.Accept()

		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return
			}

			select {
			case errChan <- err:
			default:
			}
			continue
		}

		tlsConn, ok := conn.(*tls.Conn)

		if !ok {
			conn.Close()
			continue
		}

		tlsConn.SetDeadline(time.Now().Add(l.config.HandshakeTimeout))

		if err := tlsConn.HandshakeContext(ctx); err != nil {
			tlsConn.Close()
			select {
				case errChan <- err:
				default:
			}
			continue
		}

		tlsConn.SetDeadline(time.Time{})

		select {
			case connChan <- tlsConn:
				case <-ctx.Done():
					tlsConn.Close()
					return
		}
	}


}
