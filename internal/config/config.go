package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var duration string
	if err := json.Unmarshal(b, &duration); err != nil {
		return err
	}

	parsedD, err := time.ParseDuration(duration)
	if err != nil {
		return err
	}

	*d = Duration(parsedD)

	return nil

}

func (d Duration) MarshalJSON() ([]byte, error) {
	casted := time.Duration(d).String()

	jsonified, err := json.Marshal(casted)

	return jsonified, err
}

func (d Duration) Duration() time.Duration {
	duration := time.Duration(d)

	return duration
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

type fileConfig struct {
	ListenAddress *string `json:"listen_address"`
	PeerAddress   *string `json:"peer_address"`

	TLSCertPath *string `json:"cert_path"`
	TLSKeyPath  *string `json:"key_path"`
	CAPath      *string `json:"ca_path"`
	PeerSAN     *string `json:"peer_san"`

	HandshakeTimeout *Duration `json:"handshake_timeout"`
	DialTimeout      *Duration `json:"dial_timeout"`
	IdleTimeout      *Duration `json:"idle_timeout"`
	KeepAlive        *Duration `json:"keep_alive"`

	MaxFrameSize *int `json:"max_frame_size"`
}

func DefaultConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		newConfigDir, _ := os.UserHomeDir()
		configDir = filepath.Join(newConfigDir, ".config")
	}

	return filepath.Join(configDir, "caduceus")
}

func DefaultConfig() *Config {
	baseDir := DefaultConfigDir()

	return &Config{
		ListenAddress: "",
		PeerAddress:   "",

		TLSCertPath: filepath.Join(baseDir, "cert.pem"),
		TLSKeyPath:  filepath.Join(baseDir, "key.pem"),
		CAPath:      filepath.Join(baseDir, "ca.pem"),
		PeerSAN:     "caduceus-agent",

		HandshakeTimeout: 5 * time.Second,
		DialTimeout:      5 * time.Second,
		IdleTimeout:      60 * time.Second,
		KeepAlive:        15 * time.Second,

		MaxFrameSize: 10 * 1024 * 1024,
	}
}

var (
	ErrMissingNetworkMode  = errors.New("at least one of ListenAddress or PeerAddress must be configured")
	ErrMissingCertPath     = errors.New("missing CertPath")
	ErrMissingKeyPath      = errors.New("missing KeyPath")
	ErrMissingCAPath       = errors.New("missing CAPath")
	ErrMissingPeerSAN      = errors.New("missing PeerSAN")
	ErrInvalidPEMFormat    = errors.New("invalid PEM format")
	ErrInvalidTimeout      = errors.New("invalid Timeout")
	ErrInvalidMaxFrameSize = errors.New("max frame size must be greater than zero")
)

func (c Config) Validate() error {
	if c.ListenAddress == "" && c.PeerAddress == "" {
		return fmt.Errorf("validation error: %w", ErrMissingNetworkMode)
	}

	if c.PeerAddress != "" {
		if c.PeerSAN == "" {
			return fmt.Errorf("validation error: %w", ErrMissingPeerSAN)
		}

		if c.DialTimeout <= 0 {
			return fmt.Errorf("validation error: %w", ErrInvalidTimeout)
		}

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

	if c.HandshakeTimeout <= 0 {
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
