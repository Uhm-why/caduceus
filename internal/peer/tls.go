package peer

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/Uhm-why/caduceus/internal/config"
)

var ErrInvalidPEMFormat = errors.New("invalid PEM format")

func loadCertificates(cfg *config.Config) (tls.Certificate, *x509.CertPool, error) {
	cert, err := tls.LoadX509KeyPair(cfg.TLSCertPath, cfg.TLSKeyPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("loading keypair: %w", err)
	}

	rawPEM, err := os.ReadFile(cfg.CAPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("reading CA cert: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(rawPEM) {
		return tls.Certificate{}, nil, fmt.Errorf("parsing CA certificate: %w", ErrInvalidPEMFormat)
	}

	return cert, pool, nil
}

// TLSServerConfig builds a TLS 1.3 configuration requiring client mutual TLS authentication.
func TLSServerConfig(cfg *config.Config) (*tls.Config, error) {
	cert, pool, err := loadCertificates(cfg)
	if err != nil {
		return nil, err
	}

	tlsConfig := tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}

	return &tlsConfig, nil
}

// TLSClientConfig builds a TLS 1.3 client configuration verifying the peer certificate against the CA pool and SAN.
func TLSClientConfig(cfg *config.Config) (*tls.Config, error) {
	cert, pool, err := loadCertificates(cfg)
	if err != nil {
		return nil, err
	}

	tlsConfig := tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   cfg.PeerSAN,
		MinVersion:   tls.VersionTLS13,
	}

	return &tlsConfig, nil
}
