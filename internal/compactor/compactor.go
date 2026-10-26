package compactor

import "strings"

type Config struct {
	MaxLines     int
	HeadLines    int
	TailLines    int
	SpilloverDir string
	EnableSpill  bool
}

func DefaultConfig() Config {
	return Config{
		MaxLines:    100,
		HeadLines:   30,
		TailLines:   70,
		EnableSpill: true,
	}
}

type Compactor struct {
	config Config
}

func New(cfg Config) *Compactor {
	return &Compactor{
		config: cfg,
	}
}

func (c *Compactor) Compact(input string, id string) (string, error) {

	if input == "" {
		return "", nil
	}

	cleaned := StripANSI(input)

	deduped := CollapseRepeatedLines(cleaned)

	lines := strings.Split(deduped, "\n")
	if len(lines) > c.config.MaxLines {
		return WindowAndSpill(deduped, id, c.config)
	}

	return deduped, nil
}
