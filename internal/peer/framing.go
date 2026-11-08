package peer

import (
	"encoding/json"
)

// RPCError represents a JSON-RPC 2.0 error object with code, message, and optional data.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error returns the error message, implementing the error interface.
func (e *RPCError) Error() string {
	return e.Message
}

// Request represents a JSON-RPC 2.0 request or notification wire frame.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response wire frame carrying either a result or error.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}
