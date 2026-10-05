package peer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
)

type Dialer struct {
	config *Config
}

func NewDialer(cfg *Config) *Dialer {
	return &Dialer{
		config: cfg,
	}
}

func (d *Dialer) Dial(ctx context.Context) (*tls.Conn, error) {
	intCtx, cancel := context.WithTimeout(ctx, d.config.DialTimeout)

	defer cancel()

	tlsCfg, err := d.config.TLSClientConfig()
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
