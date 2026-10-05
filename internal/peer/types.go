package peer

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"
)

type ServerState int

const (
	StateIdle ServerState = iota
	StateConnecting
	StateActive
	StateRetrying
	StateUnknown
)

var stateName = map[ServerState]string{
	StateIdle:       "idle",
	StateConnecting: "connecting",
	StateActive:     "active",
	StateRetrying:   "retrying",
	StateUnknown:    "unknown",
}

func (ss ServerState) String() string {
	if name, ok := stateName[ss]; ok {
		return name
	}

	return stateName[StateUnknown]
}

type Config struct {
	ListenAddress string
	PeerAddress   string

	TLSCertPath string
	TLSKeyPath  string
	CAPath      string
	PeerSAN     string

	HandshakeTimeout time.Duration
	DialTimeout      time.Duration
	IdleTimeout      time.Duration
	KeepAlive        time.Duration

	MaxFrameSize int
}

var ErrMissingListenAddr = errors.New("missing ListenAddress")
var ErrMissingPeerAddr = errors.New("missing PeerAddress")

var ErrMissingCertPath = errors.New("missing CertPath")
var ErrMissingKeyPath = errors.New("missing KeyPath")
var ErrMissingCAPath = errors.New("missing CAPath")
var ErrMissingPeerSAN = errors.New("missing PeerSAN")

var ErrInvalidPEMFormat = errors.New("invalid PEM format")

var ErrInvalidTimeout = errors.New("invalid Timeout")

var ErrInvalidMaxFrameSize = errors.New("max frame size must be greater than zero")

func (c Config) Validate() error {
	if c.ListenAddress == "" {
		return fmt.Errorf("validation error: %w", ErrMissingListenAddr)
	}

	if c.PeerAddress == "" {
		return fmt.Errorf("validation error: %w", ErrMissingPeerAddr)
	}

	if c.TLSCertPath == "" {
		return fmt.Errorf("validation error: %w", ErrMissingCertPath)
	}

	if c.TLSKeyPath == "" {
		return fmt.Errorf("validation error: %w", ErrMissingKeyPath)
	}

	if c.CAPath == "" {
		return fmt.Errorf("validation error: %w", ErrMissingCAPath)
	}

	if c.PeerSAN == "" {
		return fmt.Errorf("validation error: %w", ErrMissingPeerSAN)
	}

	if c.HandshakeTimeout <= 0 {
		return fmt.Errorf("validation error: %w", ErrInvalidTimeout)
	}

	if c.DialTimeout <= 0 {
		return fmt.Errorf("validation error: %w", ErrInvalidTimeout)
	}

	if c.IdleTimeout <= 0 {
		return fmt.Errorf("validation error: %w", ErrInvalidTimeout)
	}

	if c.KeepAlive <= 0 {
		return fmt.Errorf("validation error: %w", ErrInvalidTimeout)
	}

	if c.MaxFrameSize <= 0 {
		return fmt.Errorf("validation error: %w", ErrInvalidMaxFrameSize)
	}

	return nil
}

func (c *Config) loadCertificates() (tls.Certificate, *x509.CertPool, error) {
	cert, err := tls.LoadX509KeyPair(c.TLSCertPath, c.TLSKeyPath)

	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("loading keypair: %w", err)
	}

	rawPEM, err := os.ReadFile(c.CAPath)

	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("reading CA cert: %w", err)
	}

	pool := x509.NewCertPool()

	poolCheck := pool.AppendCertsFromPEM(rawPEM)

	if poolCheck == false {
		return tls.Certificate{}, nil, fmt.Errorf("parsing CA certificate: %w", ErrInvalidPEMFormat)
	}

	return cert, pool, nil
}

func (c *Config) TLSServerConfig() (*tls.Config, error) {
	cert, pool, err := c.loadCertificates()

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

func (c *Config) TLSClientConfig() (*tls.Config, error) {
	cert, pool, err := c.loadCertificates()

	if err != nil {
		return nil, err
	}

	tlsConfig := tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   c.PeerSAN,
		MinVersion:   tls.VersionTLS13,
	}

	return &tlsConfig, nil
}
