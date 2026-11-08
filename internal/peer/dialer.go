package peer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/Uhm-why/caduceus/internal/config"
)

// Dialer establishes outbound mTLS 1.3 connections to a configured peer.
type Dialer struct {
	config *config.Config
}

// NewDialer constructs a Dialer configured with cfg.
func NewDialer(cfg *config.Config) *Dialer {
	return &Dialer{
		config: cfg,
	}
}

// Dial initiates a TLS connection to cfg.PeerAddress within the configured DialTimeout.
func (d *Dialer) Dial(ctx context.Context) (*tls.Conn, error) {
	intCtx, cancel := context.WithTimeout(ctx, d.config.DialTimeout)

	defer cancel()

	tlsCfg, err := TLSClientConfig(d.config)
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{
		KeepAlive: d.config.KeepAlive,
	}

	tlsDialer := &tls.Dialer{
		NetDialer: dialer,
		Config:    tlsCfg,
	}

	rawConn, err := tlsDialer.DialContext(intCtx, "tcp", d.config.PeerAddress)

	if err != nil {
		return nil, fmt.Errorf("dialer peer %s: %w", d.config.PeerAddress, err)
	}

	tlsConn, ok := rawConn.(*tls.Conn)
	if !ok {
		rawConn.Close()
		return nil, fmt.Errorf("connection is not *tls.Conn")
	}

	return tlsConn, nil
}
