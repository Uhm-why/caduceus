// Package peer provides bidirectional mTLS 1.3 transport, JSON-RPC wire framing,
// capability negotiation, consultative and task delegation lifecycles, and a stdio
// Model Context Protocol (MCP) bridge.
package peer

// ServerState represents the operational connection state of a peer session.
type ServerState int

// Lifecycle states for a peer connection.
const (
	StateIdle       ServerState = iota // No active network connection.
	StateConnecting                    // Outbound connection attempt in progress.
	StateHandshake                     // TLS established, negotiating capabilities.
	StateActive                        // Handshake verified, ready for traffic.
	StateRetrying                      // Connection dropped, awaiting retry.
	StateUnknown                       // Unrecognized or invalid state.
)

var stateName = map[ServerState]string{
	StateIdle:       "idle",
	StateConnecting: "connecting",
	StateHandshake:  "handshake",
	StateActive:     "active",
	StateRetrying:   "retrying",
	StateUnknown:    "unknown",
}

// String returns the string representation of the ServerState.
func (ss ServerState) String() string {
	if name, ok := stateName[ss]; ok {
		return name
	}

	return stateName[StateUnknown]
}
