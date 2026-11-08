package peer

import (
	"os"
	"runtime"
)

// Capabilities defines the protocol version, agent identifier, supported features,
// and host metadata exchanged during peer handshake negotiation.
type Capabilities struct {
	Version  string            `json:"version"`
	Agent    string            `json:"agent"`
	Features []string          `json:"features"`
	Metadata map[string]string `json:"metadata"`
}

// DefaultCapabilities returns the standard node capabilities: version 1.0.0,
// agent name "caduceus-agent", messaging/consultative/tasks features, and runtime OS/Arch/Host info.
func DefaultCapabilities() *Capabilities {
	hostname, _ := os.Hostname()
	return &Capabilities{
		Version:  "1.0.0",
		Agent:    "caduceus-agent",
		Features: []string{"messaging", "consultative", "tasks"},
		Metadata: map[string]string{
			"os":   runtime.GOOS,
			"arch": runtime.GOARCH,
			"host": hostname,
		},
	}
}

// HasFeature reports whether the given feature name is present in c.Features.
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
