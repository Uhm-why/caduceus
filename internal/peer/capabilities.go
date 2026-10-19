package peer

import (
	"os"
	"runtime"
)

type Capabilities struct {
	Version  string            `json:"version"`
	Agent    string            `json:"agent"`
	Features []string          `json:"features"`
	Metadata map[string]string `json:"metadata"`
}

func DefaultCapabilities() *Capabilities {
	hostname, _ := os.Hostname()
	return &Capabilities{
		Version:  "1.0.0",
		Agent:    "caduceus-agent",
		Features: []string{"messaging", "consultative"},
		Metadata: map[string]string{
			"os":   runtime.GOOS,
			"arch": runtime.GOARCH,
			"host": hostname,
		},
	}
}

func (c *Capabilities) HasFeature(name string) bool {
	if c == nil {
		return false
	}

	for _, feature := range c.Features {
		if feature == name {
			return true
		}
	}

	return false
}
