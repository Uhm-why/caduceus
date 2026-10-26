package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/Uhm-why/caduceus/internal/compactor"
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

type fileCompactorConfig struct {
	MaxLines     *int    `json:"max_lines"`
	HeadLines    *int    `json:"head_lines"`
	TailLines    *int    `json:"tail_lines"`
	SpilloverDir *string `json:"spillover_dir"`
	EnableSpill  *bool   `json:"enable_spill"`
}

type Config struct {
	ListenAddress string
	PeerAddress   string

	TLSCertPath string
	TLSKeyPath  string
	CAPath      string
	PeerSAN     string
	MailboxPath string

	HandshakeTimeout time.Duration
	DialTimeout      time.Duration
	IdleTimeout      time.Duration
	KeepAlive        time.Duration

	MaxFrameSize int
	Compactor    compactor.Config
}

type fileConfig struct {
	ListenAddress *string `json:"listen_address"`
	PeerAddress   *string `json:"peer_address"`

	TLSCertPath *string `json:"cert_path"`
	TLSKeyPath  *string `json:"key_path"`
	CAPath      *string `json:"ca_path"`
	PeerSAN     *string `json:"peer_san"`
	MailboxPath *string `json:"mailbox_path"`

	HandshakeTimeout *Duration `json:"handshake_timeout"`
	DialTimeout      *Duration `json:"dial_timeout"`
	IdleTimeout      *Duration `json:"idle_timeout"`
	KeepAlive        *Duration `json:"keep_alive"`

	MaxFrameSize *int                 `json:"max_frame_size"`
	Compactor    *fileCompactorConfig `json:"compactor"`
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
	compactorCfg := compactor.DefaultConfig()
	compactorCfg.SpilloverDir = filepath.Join(baseDir, "spillover")

	return &Config{
		ListenAddress: "",
		PeerAddress:   "",

		TLSCertPath: filepath.Join(baseDir, "cert.pem"),
		TLSKeyPath:  filepath.Join(baseDir, "key.pem"),
		CAPath:      filepath.Join(baseDir, "ca.pem"),
		PeerSAN:     "caduceus-agent",
		MailboxPath: filepath.Join(baseDir, "mailbox"),

		HandshakeTimeout: 5 * time.Second,
		DialTimeout:      5 * time.Second,
		IdleTimeout:      60 * time.Second,
		KeepAlive:        15 * time.Second,

		MaxFrameSize: 10 * 1024 * 1024,
		Compactor:    compactorCfg,
	}
}

var (
	ErrMissingNetworkMode    = errors.New("at least one of ListenAddress or PeerAddress must be configured")
	ErrMissingCertPath       = errors.New("missing CertPath")
	ErrMissingKeyPath        = errors.New("missing KeyPath")
	ErrMissingCAPath         = errors.New("missing CAPath")
	ErrMissingPeerSAN        = errors.New("missing PeerSAN")
	ErrInvalidPEMFormat      = errors.New("invalid PEM format")
	ErrMissingMailboxPath    = errors.New("missing MailboxPath")
	ErrInvalidTimeout        = errors.New("invalid Timeout")
	ErrInvalidMaxFrameSize   = errors.New("max frame size must be greater than zero")
	ErrInvalidCompactorLines = errors.New("compactor max lines must be greater than zero and greater than head plus tail lines")
	ErrMissingSpilloverDir   = errors.New("missing compactor spillover dir when spillover is enabled")
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

	if c.MailboxPath == "" {
		return fmt.Errorf("validation error: %w", ErrMissingMailboxPath)
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

	if c.Compactor.MaxLines <= 0 {
		return fmt.Errorf("validation error: %w", ErrInvalidCompactorLines)
	}

	if c.Compactor.HeadLines < 0 || c.Compactor.TailLines < 0 {
		return fmt.Errorf("validation error: %w", ErrInvalidCompactorLines)
	}

	if (c.Compactor.HeadLines + c.Compactor.TailLines) >= c.Compactor.MaxLines {
		return fmt.Errorf("validation error: %w", ErrInvalidCompactorLines)
	}
	if c.Compactor.EnableSpill {
		if c.Compactor.SpilloverDir == "" {
			return fmt.Errorf("validation error: %w", ErrMissingSpilloverDir)
		}
	}

	return nil
}

func ParseAndLoad(args []string) (*Config, error) {
	flags := flag.NewFlagSet("caduceus", flag.ContinueOnError)
	listenFlag := flags.String("listen", "", "Address to bind for incoming peer connections (e.g. :8443).")
	ipFlag := flags.String("ip", "", "The IP and port [0.0.0.0:0000] of the other machine running caduceus to connect to.")
	configFlag := flags.String("config", "", "The directory or file path where caduceus configuration is stored.")
	mailboxFlag := flags.String("mailbox-path", "", "The directory where caduceus persists queued messages.")
	sanFlag := flags.String("san", "", "Expected Subject Alternative Name (SAN)")
	handshakeTimeoutFlag := flags.String("handshake-timeout", "", "The amount of time (in seconds) the agent will wait on a handshake before aborting.")
	dialTimeoutFlag := flags.String("dial-timeout", "", "The amount of time the agent will spend trying to dial a peer before aborting.")
	idleTimeoutFlag := flags.String("idle-timeout", "", "The amount of time the agent will wait with no activity before aborting.")
	keepAliveFlag := flags.String("keepalive", "", "The set keepalive for the connection.")
	maxFrameSizeFlag := flags.Int("max-frame-size", 0, "The maximum size of a frame to send to another agent.")

	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	var err error

	explicit := make(map[string]bool)

	flags.Visit(func(f *flag.Flag) {
		explicit[f.Name] = true
	})

	filePath, baseDir, found := ResolveConfigFile(*configFlag)
	if baseDir != DefaultConfigDir() {
		cfg.TLSCertPath = filepath.Join(baseDir, "cert.pem")
		cfg.TLSKeyPath = filepath.Join(baseDir, "key.pem")
		cfg.CAPath = filepath.Join(baseDir, "ca.pem")
		cfg.MailboxPath = filepath.Join(baseDir, "mailbox")
		cfg.Compactor.SpilloverDir = filepath.Join(baseDir, "spillover")
	}

	if found {
		if err := LoadFile(filePath, cfg); err != nil {
			return nil, err
		}
	} else if explicit["config"] && filepath.Ext(*configFlag) == ".json" {
		return nil, fmt.Errorf("config file %q not found", *configFlag)
	}

	if explicit["listen"] {
		cfg.ListenAddress = *listenFlag
	}
	if explicit["ip"] {
		cfg.PeerAddress = *ipFlag
	}
	if explicit["san"] {
		cfg.PeerSAN = *sanFlag
	}
	if explicit["mailbox-path"] {
		cfg.MailboxPath = *mailboxFlag
	}

	if explicit["handshake-timeout"] {

		cfg.HandshakeTimeout, err = time.ParseDuration(*handshakeTimeoutFlag)
		if err != nil {
			return nil, fmt.Errorf("time related flags should be in the golang duration format. (e.g. `5s` for 5 seconds). Please retry the command with: %s error: %w", *handshakeTimeoutFlag, err)
		}
	}
	if explicit["dial-timeout"] {
		cfg.DialTimeout, err = time.ParseDuration(*dialTimeoutFlag)
		if err != nil {
			return nil, fmt.Errorf("time related flags should be in the golang duration format. (e.g. `5s` for 5 seconds). Please retry the command with: %s error: %w", *dialTimeoutFlag, err)
		}
	}
	if explicit["idle-timeout"] {
		cfg.IdleTimeout, err = time.ParseDuration(*idleTimeoutFlag)
		if err != nil {
			return nil, fmt.Errorf("time related flags should be in the golang duration format. (e.g. `5s` for 5 seconds). Please retry the command with: %s error: %w", *idleTimeoutFlag, err)
		}
	}
	if explicit["keepalive"] {
		cfg.KeepAlive, err = time.ParseDuration(*keepAliveFlag)
		if err != nil {
			return nil, fmt.Errorf("time related flags should be in the golang duration format for seconds. (e.g. `5s` for 5 seconds). Please retry the command with: %s error: %w", *keepAliveFlag, err)
		}
	}
	if explicit["max-frame-size"] {
		cfg.MaxFrameSize = *maxFrameSizeFlag
	}

	if cfg.PeerAddress != "" {
		_, _, err := net.SplitHostPort(cfg.PeerAddress)
		if err != nil {
			return nil, err
		}
	}

	err = cfg.Validate()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
