package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ResolveConfigFile determines the config.json path and base directory from customPath
// or standard user configuration paths, returning the file path, base directory, and
// whether the file exists on disk.
func ResolveConfigFile(customPath string) (filePath string, baseDir string, found bool) {
	if customPath != "" {
		info, err := os.Stat(customPath)
		if err == nil && info.IsDir() {
			candidate := filepath.Join(customPath, "config.json")
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				return candidate, customPath, true
			}
			return "", customPath, false
		}

		if err == nil && !info.IsDir() {
			return customPath, filepath.Dir(customPath), true
		}

		if err != nil {
			if filepath.Ext(customPath) == ".json" {
				return customPath, filepath.Dir(customPath), false
			}

			return filepath.Join(customPath, "config.json"), customPath, false
		}
	}

	if customPath == "" {
		var candidates []string
		candidates = append(candidates, filepath.Join(DefaultConfigDir(), "config.json"))
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, ".caduceus", "config.json"))
		}

		for _, candidate := range candidates {
			_, err := os.Stat(candidate)
			if err == nil {
				return candidate, filepath.Dir(candidate), true
			}
		}

	}

	return "", DefaultConfigDir(), false
}

// LoadFile reads and unmarshals config.json from path, overlaying defined JSON values
// onto cfg.
func LoadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("parsing config file %s: %w", path, err)
	}

	var fc fileConfig

	err = json.Unmarshal(data, &fc)
	if err != nil {
		return fmt.Errorf("parsing config file %s: %w", path, err)
	}

	if fc.ListenAddress != nil {
		cfg.ListenAddress = *fc.ListenAddress
	}
	if fc.PeerAddress != nil {
		cfg.PeerAddress = *fc.PeerAddress
	}
	if fc.TLSCertPath != nil {
		cfg.TLSCertPath = *fc.TLSCertPath
	}
	if fc.TLSKeyPath != nil {
		cfg.TLSKeyPath = *fc.TLSKeyPath
	}
	if fc.CAPath != nil {
		cfg.CAPath = *fc.CAPath
	}
	if fc.PeerSAN != nil {
		cfg.PeerSAN = *fc.PeerSAN
	}
	if fc.MailboxPath != nil {
		cfg.MailboxPath = *fc.MailboxPath
	}
	if fc.HandshakeTimeout != nil {
		cfg.HandshakeTimeout = fc.HandshakeTimeout.Duration()
	}
	if fc.DialTimeout != nil {
		cfg.DialTimeout = fc.DialTimeout.Duration()
	}
	if fc.IdleTimeout != nil {
		cfg.IdleTimeout = fc.IdleTimeout.Duration()
	}
	if fc.KeepAlive != nil {
		cfg.KeepAlive = fc.KeepAlive.Duration()
	}
	if fc.MaxFrameSize != nil {
		cfg.MaxFrameSize = *fc.MaxFrameSize
	}
	if fc.Compactor != nil {
		if fc.Compactor.MaxLines != nil {
			cfg.Compactor.MaxLines = *fc.Compactor.MaxLines
		}
		if fc.Compactor.HeadLines != nil {
			cfg.Compactor.HeadLines = *fc.Compactor.HeadLines
		}
		if fc.Compactor.TailLines != nil {
			cfg.Compactor.TailLines = *fc.Compactor.TailLines
		}
		if fc.Compactor.SpilloverDir != nil {
			cfg.Compactor.SpilloverDir = *fc.Compactor.SpilloverDir
		}
		if fc.Compactor.EnableSpill != nil {
			cfg.Compactor.EnableSpill = *fc.Compactor.EnableSpill
		}
	}

	return nil
}
