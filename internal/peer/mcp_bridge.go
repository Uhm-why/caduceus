package peer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

type Bridge struct {
	session  *Session
	dialer   *Dialer
	stdoutMu sync.Mutex
	dialMu   sync.Mutex
}

func NewBridge(s *Session, d *Dialer) *Bridge {
	return &Bridge{
		session: s,
		dialer:  d,
	}
}

type StdioRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type StdioResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

func (b *Bridge) writeResponse(resp any) error {
	marshResp, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshalling json: %w", ErrMarshalling)
	}

	frame := append(marshResp, '\n')
	b.stdoutMu.Lock()
	os.Stdout.Write(frame)
	b.stdoutMu.Unlock()

	return nil
}

type PropertyDefinition struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ToolInputSchema struct {
	Type       string                        `json:"type"`
	Properties map[string]PropertyDefinition `json:"properties"`
	Required   []string                      `json:"required,omitempty"`
}

type ToolDescriptor struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema ToolInputSchema `json:"inputSchema"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ServerCapes struct {
	Tools map[string]any `json:"tools"`
}

type ServerMetaData struct {
	ProtocolVersion   string      `json:"protocolVersion"`
	Capabilities      ServerCapes `json:"capabilities"`
	ServerInformation ServerInfo  `json:"serverInfo"`
}

var agentPeerSendTool = ToolDescriptor{
	Name:        "agent_peer_send",
	Description: "Send a message or task to the connected peer agent",
	InputSchema: ToolInputSchema{
		Type: "object",
		Properties: map[string]PropertyDefinition{
			"message": {
				Type:        "string",
				Description: "The content to transmit to the peer",
			},
		},
		Required: []string{"message"},
	},
}

var agentPeerStatusTool = ToolDescriptor{
	Name:        "agent_peer_status",
	Description: "Check the active mTLS connection state to the peer agent",
	InputSchema: ToolInputSchema{
		Type:       "object",
		Properties: map[string]PropertyDefinition{},
	},
}

var agentPeerInboxTool = ToolDescriptor{
	Name:        "agent_peer_inbox",
	Description: "Check inbox for messages or tasks from a peer agent",
	InputSchema: ToolInputSchema{
		Type:       "object",
		Properties: map[string]PropertyDefinition{},
	},
}

var agentPeerCapabilitiesTool = ToolDescriptor{
	Name:        "agent_peer_capabilities",
	Description: "Inspect the negotiated capabilities, features, and metadata of the connected peer agent",
	InputSchema: ToolInputSchema{
		Type:       "object",
		Properties: map[string]PropertyDefinition{},
	},
}

func (b *Bridge) ensureConnected(ctx context.Context) error {
	b.dialMu.Lock()
	defer b.dialMu.Unlock()
	if b.session.State() != StateIdle {
		return nil
	}

	conn, err := b.dialer.Dial(ctx)
	if err != nil {
		return err
	}

	if err := b.session.Attach(ctx, conn); err != nil && !errors.Is(err, ErrSessionActive) {
		return err
	}

	if errors.Is(err, ErrSessionActive) {
		return nil
	}

	if err := b.session.PerformHandshake(ctx); err != nil {
		return err
	}

	return nil

}

func (b *Bridge) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 10485760)

	for scanner.Scan() {
		line := scanner.Bytes()
		var req StdioRequest

		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		switch req.Method {
		case "initialize":
			initData := ServerMetaData{
				ProtocolVersion: "2024-11-05",
				Capabilities: ServerCapes{
					Tools: map[string]any{},
				},
				ServerInformation: ServerInfo{
					Name:    "caduceus",
					Version: "1.0.0",
				},
			}

			b.writeResponse(StdioResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  initData,
			})
		case "notifications/initialized":

		case "tools/list":
			b.writeResponse(StdioResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"tools": []ToolDescriptor{
						agentPeerSendTool,
						agentPeerStatusTool,
						agentPeerInboxTool,
						agentPeerCapabilitiesTool,
					},
				},
			})

		case "tools/call":
			var callParams struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}

			_ = json.Unmarshal(req.Params, &callParams)

			if callParams.Name == "agent_peer_status" {
				statusText := fmt.Sprintf("Session status: %s", b.session.State().String())

				b.writeResponse(StdioResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": statusText},
						},
					},
				})
			}

			if callParams.Name == "agent_peer_send" {
				if b.session.State() == StateIdle {
					if err := b.ensureConnected(ctx); err != nil {
						b.writeResponse(StdioResponse{
							JSONRPC: "2.0",
							ID:      req.ID,
							Result: map[string]any{
								"isError": true,
								"content": []map[string]any{
									{"type": "text", "text": fmt.Sprintf("Connection failed: %v", err)},
								},
							},
						})
						break
					}
				}

				if !b.session.HasRemoteFeature("messaging") {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Peer does not support the 'messaging' capability")},
							},
						},
					})
					break
				}

				res, err := b.session.SendRequest(ctx, "agent/message", callParams.Arguments)
				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Send error: %v", err)},
							},
						},
					})
					break
				}
				b.writeResponse(StdioResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": string(res)},
						},
					},
				})
			}

			if callParams.Name == "agent_peer_inbox" {
				messages, err := b.session.DrainInbox()
				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Error draining inbox: %v", err)},
							},
						},
					})
					break
				}

				data, err := json.MarshalIndent(messages, "", "  ")

				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Error marshalling inbox: %v", err)},
							},
						},
					})
					break
				}

				b.writeResponse(StdioResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": string(data)},
						},
					},
				})
			}

			if callParams.Name == "agent_peer_capabilities" {
				if b.session.State() == StateIdle {
					if err := b.ensureConnected(ctx); err != nil {
						b.writeResponse(StdioResponse{
							JSONRPC: "2.0",
							ID:      req.ID,
							Result: map[string]any{
								"isError": true,
								"content": []map[string]any{
									{"type": "text", "text": fmt.Sprintf("Session StateIdle and failed to connect: %v", err)},
								},
							},
						})
						break
					}
				}

				if b.session.State() != StateActive {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Peer connection not active (current state: %s)", b.session.State().String())},
							},
						},
					})
					break
				}

				caps := b.session.RemoteCapabilities()

				if caps == nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"content": []map[string]any{
								{"type": "text", "text": string("No remote capabilities negotiated")},
							},
						},
					})
					break
				}

				data, err := json.MarshalIndent(caps, "", "  ")

				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Marshalling capabilities: %v", err)},
							},
						},
					})
					break
				}

				b.writeResponse(StdioResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": string(data)},
						},
					},
				})

			}

		default:
			if req.ID != nil {
				b.writeResponse(StdioResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: map[string]any{
						"code":    -32601,
						"message": "method not found",
					},
				})
			}
		}

	}

	return scanner.Err()
}
