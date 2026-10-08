package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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

	return nil
}
