package peer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"sync"
	"time"
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

var agentPeerConsultTool = ToolDescriptor{
	Name:        "agent_peer_consult",
	Description: "Consult a peer agent for a question or guidance",
	InputSchema: ToolInputSchema{
		Type: "object",
		Properties: map[string]PropertyDefinition{
			"query": {
				Type:        "string",
				Description: "Specific advice, question, or guidance requested",
			},
			"context": {
				Type:        "string",
				Description: "Background constraints, code snippets, or system logs",
			},
		},
		Required: []string{"query"},
	},
}

var agentPeerReviewTool = ToolDescriptor{
	Name:        "agent_peer_review",
	Description: "Request review of code diffs, configuration files, or architectural text",
	InputSchema: ToolInputSchema{
		Type: "object",
		Properties: map[string]PropertyDefinition{
			"artifact": {
				Type:        "string",
				Description: "Code diff, configuration file, or architectural text",
			},
			"rubric": {
				Type:        "string",
				Description: "Review criteria (e.g. \"memory leaks\", \"security\", \"style\")",
			},
		},
		Required: []string{"artifact"},
	},
}

var agentPeerReplyTool = ToolDescriptor{
	Name:        "agent_peer_reply",
	Description: "Response to a peer agent's consult or review request",
	InputSchema: ToolInputSchema{
		Type: "object",
		Properties: map[string]PropertyDefinition{
			"ticket_id": {
				Type:        "string",
				Description: "The correlation ID of the consult or review request",
			},
			"response": {
				Type:        "string",
				Description: "The synthesized advice, answer, or code review findings",
			},
			"status": {
				Type:        "string",
				Description: "\"completed\", \"rejected\", \"need_more_info\" (defaults to \"completed\")",
			},
		},
		Required: []string{"ticket_id", "response"},
	},
}

var agentPeerDelegateTool = ToolDescriptor{
	Name:        "agent_peer_delegate",
	Description: "Delegate a task or job to the connected peer agent for execution",
	InputSchema: ToolInputSchema{
		Type: "object",
		Properties: map[string]PropertyDefinition{
			"objective": {
				Type:        "string",
				Description: "The task or job to the connected peer agent for execution",
			},
			"context": {
				Type:        "string",
				Description: "Background details, file paths, code snippets, or constraints",
			},
			"timeout_sec": {
				Type:        "integer",
				Description: "Timeout in seconds for task execution (defaults to 60)",
			},
			"task_id": {
				Type:        "string",
				Description: "Unique task identifier (auto-generated if omitted)",
			},
		},
		Required: []string{"objective"},
	},
}

var agentPeerTaskStatusTool = ToolDescriptor{
	Name:        "agent_peer_task_status",
	Description: "Query the status, progress, result, and artifacts of a delegated task",
	InputSchema: ToolInputSchema{
		Type: "object",
		Properties: map[string]PropertyDefinition{
			"task_id": {
				Type:        "string",
				Description: "The unique task identifier to query",
			},
		},
		Required: []string{"task_id"},
	},
}

var agentPeerTaskCancelTool = ToolDescriptor{
	Name:        "agent_peer_task_cancel",
	Description: "Cancel or abort an active task running on the peer agent",
	InputSchema: ToolInputSchema{
		Type: "object",
		Properties: map[string]PropertyDefinition{
			"task_id": {
				Type:        "string",
				Description: "The unique task identifier to cancel",
			},
			"reason": {
				Type:        "string",
				Description: "Reason for cancellation",
			},
		},
		Required: []string{"task_id"},
	},
}

var agentPeerTaskCompleteTool = ToolDescriptor{
	Name:        "agent_peer_task_complete",
	Description: "Mark an active delegated task as completed with results and optional artifacts",
	InputSchema: ToolInputSchema{
		Type: "object",
		Properties: map[string]PropertyDefinition{
			"task_id": {
				Type:        "string",
				Description: "The unique task identifier to complete",
			},
			"result": {
				Type:        "string",
				Description: "The execution output, telemetry, or result of the completed task",
			},
			"artifacts": {
				Type:        "array",
				Description: "Optional list of file paths or artifacts produced by the task",
			},
		},
		Required: []string{"task_id", "result"},
	},
}

var agentPeerTaskProgressTool = ToolDescriptor{
	Name:        "agent_peer_task_progress",
	Description: "Update progress notes on an active delegated task",
	InputSchema: ToolInputSchema{
		Type: "object",
		Properties: map[string]PropertyDefinition{
			"task_id": {
				Type:        "string",
				Description: "The unique task identifier",
			},
			"progress": {
				Type:        "string",
				Description: "Progress update message or milestone",
			},
		},
		Required: []string{"task_id", "progress"},
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
						agentPeerConsultTool,
						agentPeerReviewTool,
						agentPeerReplyTool,
						agentPeerDelegateTool,
						agentPeerTaskStatusTool,
						agentPeerTaskCancelTool,
						agentPeerTaskCompleteTool,
						agentPeerTaskProgressTool,
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

				if b.session.Compactor() != nil {
					for i := range messages {
						if compacted, err := b.session.Compactor().Compact(messages[i].Content, messages[i].ID); err == nil {
							messages[i].Content = compacted
						}
					}
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
			if callParams.Name == "agent_peer_consult" {
				if err := b.ensureConnected(ctx); err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Failed to connect: %v", err)},
							},
						},
					})
					break
				}

				if !b.session.HasRemoteFeature("consultative") {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Peer agent does not have consultative capability"},
							},
						},
					})
					break
				}

				var inboundArgs struct {
					Query   string `json:"query"`
					Context string `json:"context,omitempty"`
				}
				var peerSessionInbound struct {
					CorrelationID string `json:"correlation_id"`
					Kind          string `json:"kind"`
					Message       string `json:"message"`
				}

				_ = json.Unmarshal(callParams.Arguments, &inboundArgs)

				if inboundArgs.Query == "" {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Query is a required value"},
							},
						},
					})
					break
				}

				peerSessionInbound.CorrelationID = fmt.Sprintf("tkt-%d-%d", time.Now().UTC().UnixNano(), rand.Int())
				peerSessionInbound.Kind = "consult_request"

				if inboundArgs.Context != "" {
					peerSessionInbound.Message = fmt.Sprintf("%s\n%s", inboundArgs.Query, inboundArgs.Context)
				} else {
					peerSessionInbound.Message = fmt.Sprintf("%s", inboundArgs.Query)
				}

				payloadBytes, err := json.Marshal(peerSessionInbound)
				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("error marshalling json: %v", err)},
							},
						},
					})
					break
				}

				_, err = b.session.SendRequest(ctx, "agent/message", payloadBytes)
				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("failed to send consultation: %v", err)},
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
							{"type": "text", "text": fmt.Sprintf("Consultation enqueued to peer. Check agent_peer_inbox for matching ticket id. Ticket ID: %s.", peerSessionInbound.CorrelationID)},
						},
					},
				})
			}

			if callParams.Name == "agent_peer_review" {
				if err := b.ensureConnected(ctx); err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Failed to connect: %v", err)},
							},
						},
					})
					break
				}

				if !b.session.HasRemoteFeature("consultative") {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Peer agent does not have consultative capability"},
							},
						},
					})
					break
				}

				var inboundArgs struct {
					Artifact string `json:"artifact"`
					Rubric   string `json:"rubric,omitempty"`
				}

				_ = json.Unmarshal(callParams.Arguments, &inboundArgs)

				if inboundArgs.Artifact == "" {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Artifact is a required value"},
							},
						},
					})
					break
				}

				var peerSessionInbound struct {
					CorrelationID string `json:"correlation_id"`
					Kind          string `json:"kind"`
					Message       string `json:"message"`
				}

				peerSessionInbound.CorrelationID = fmt.Sprintf("tkt-%d-%d", time.Now().UTC().UnixNano(), rand.Int())
				peerSessionInbound.Kind = "review_request"
				if inboundArgs.Rubric != "" {
					peerSessionInbound.Message = fmt.Sprintf("%s\n\nRubric:\n%s", inboundArgs.Artifact, inboundArgs.Rubric)
				} else {
					peerSessionInbound.Message = fmt.Sprintf("%s", inboundArgs.Artifact)
				}

				payloadBytes, err := json.Marshal(peerSessionInbound)
				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("error marshalling json: %v", err)},
							},
						},
					})
					break
				}

				_, err = b.session.SendRequest(ctx, "agent/message", payloadBytes)
				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("failed to send review request: %v", err)},
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
							{"type": "text", "text": fmt.Sprintf("Review Request enqueued to peer. Check agent_peer_inbox for matching ticket id. Ticket ID: %s.", peerSessionInbound.CorrelationID)},
						},
					},
				})

			}

			if callParams.Name == "agent_peer_reply" {
				if err := b.ensureConnected(ctx); err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Failed to connect: %v", err)},
							},
						},
					})
					break
				}

				if !b.session.HasRemoteFeature("consultative") {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Peer agent does not have consultative capability"},
							},
						},
					})
					break
				}

				var inboundArgs struct {
					TicketID string `json:"ticket_id"`
					Response string `json:"response"`
					Status   string `json:"status,omitempty"`
				}

				_ = json.Unmarshal(callParams.Arguments, &inboundArgs)

				if inboundArgs.TicketID == "" || inboundArgs.Response == "" {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Ticket ID and Response are required values"},
							},
						},
					})
					break
				}

				var peerSessionInbound struct {
					Status        string `json:"status"`
					CorrelationID string `json:"correlation_id"`
					Kind          string `json:"kind"`
					Message       string `json:"message"`
				}

				if inboundArgs.Status == "" {
					peerSessionInbound.Status = "completed"
				} else {
					peerSessionInbound.Status = inboundArgs.Status
				}

				peerSessionInbound.CorrelationID = inboundArgs.TicketID
				peerSessionInbound.Kind = "reply"
				peerSessionInbound.Message = fmt.Sprintf("[%s] %s", peerSessionInbound.Status, inboundArgs.Response)

				payloadBytes, err := json.Marshal(peerSessionInbound)
				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("error marshalling json: %v", err)},
							},
						},
					})
					break
				}

				_, err = b.session.SendRequest(ctx, "agent/message", payloadBytes)
				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("failed to send reply: %v", err)},
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
							{"type": "text", "text": fmt.Sprintf("Reply enqueued to peer. Check agent_peer_inbox for matching ticket id. Ticket ID: %s.", peerSessionInbound.CorrelationID)},
						},
					},
				})

			}

			if callParams.Name == "agent_peer_delegate" {
				if err := b.ensureConnected(ctx); err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Failed to connect: %v", err)},
							},
						},
					})
					break
				}

				if !b.session.HasRemoteFeature("tasks") {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Peer agent does not have tasks capability"},
							},
						},
					})
					break
				}

				var taskArgs struct {
					TaskID     string `json:"task_id"`
					Objective  string `json:"objective"`
					Context    string `json:"context,omitempty"`
					TimeoutSec int    `json:"timeout_sec,omitempty"`
				}

				_ = json.Unmarshal(callParams.Arguments, &taskArgs)

				if taskArgs.Objective == "" {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Objective is a required value"},
							},
						},
					})
					break
				}

				if taskArgs.TaskID == "" {
					taskArgs.TaskID = fmt.Sprintf("task-%d-%d", time.Now().UTC().UnixNano(), rand.Int())
				}

				if taskArgs.TimeoutSec <= 0 {
					taskArgs.TimeoutSec = 60
				}

				dispatchRes, err := b.session.DispatchTask(ctx, TaskDispatchRequest{
					TaskID:     taskArgs.TaskID,
					Objective:  taskArgs.Objective,
					Context:    taskArgs.Context,
					TimeoutSec: taskArgs.TimeoutSec,
				})
				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Error dispatching task: %v", err)},
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
							{"type": "text", "text": fmt.Sprintf("Task delegated to peer. Task ID: %s, Accepted: %t, Initial State: %s. Use agent_peer_task_status to monitor progress.", dispatchRes.TaskID, dispatchRes.Accepted, dispatchRes.InitialState)},
						},
					},
				})

			}

			if callParams.Name == "agent_peer_task_status" {
				if err := b.ensureConnected(ctx); err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Failed to connect: %v", err)},
							},
						},
					})
					break
				}

				if !b.session.HasRemoteFeature("tasks") {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Peer agent does not have tasks capability"},
							},
						},
					})
					break
				}

				var statusArgs struct {
					TaskID string `json:"task_id"`
				}

				_ = json.Unmarshal(callParams.Arguments, &statusArgs)

				if statusArgs.TaskID == "" {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Task ID is a required value"},
							},
						},
					})
					break
				}

				statusRes, err := b.session.QueryTaskStatus(ctx, statusArgs.TaskID)
				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Error querying task status: %v", err)},
							},
						},
					})
					break
				}

				displayResult := statusRes.Result
				if b.session.Compactor() != nil && displayResult != "" {
					if compacted, err := b.session.Compactor().Compact(displayResult, statusRes.TaskID); err == nil {
						displayResult = compacted
					}
				}

				statusText := fmt.Sprintf("Task ID: %s\nState: %s\nProgress: %s\nResult: %s\nArtifacts: %v\nError: %s\nElapsed: %ds",
					statusRes.TaskID,
					statusRes.State,
					statusRes.Progress,
					displayResult,
					statusRes.Artifacts,
					statusRes.Error,
					statusRes.ElapsedSec,
				)

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

			if callParams.Name == "agent_peer_task_cancel" {
				if err := b.ensureConnected(ctx); err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Failed to connect: %v", err)},
							},
						},
					})
					break
				}

				if !b.session.HasRemoteFeature("tasks") {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Peer agent does not have tasks capability"},
							},
						},
					})
					break
				}

				var cancelArgs struct {
					TaskID string `json:"task_id"`
					Reason string `json:"reason,omitempty"`
				}

				_ = json.Unmarshal(callParams.Arguments, &cancelArgs)

				if cancelArgs.TaskID == "" {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "Task ID is a required value"},
							},
						},
					})
					break
				}

				cancelRes, err := b.session.CancelTask(ctx, cancelArgs.TaskID, cancelArgs.Reason)
				if err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Error canceling task: %v", err)},
							},
						},
					})
					break
				}

				cancelText := fmt.Sprintf("Task ID: %s, Canceled: %t, Final State: %s",
					cancelRes.TaskID,
					cancelRes.Canceled,
					cancelRes.FinalState,
				)

				b.writeResponse(StdioResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": cancelText},
						},
					},
				})

			}

			if callParams.Name == "agent_peer_task_progress" {
				var progressArgs struct {
					TaskID   string `json:"task_id"`
					Progress string `json:"progress"`
				}

				_ = json.Unmarshal(callParams.Arguments, &progressArgs)

				if progressArgs.TaskID == "" || progressArgs.Progress == "" {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "task_id and progress are required values"},
							},
						},
					})
					break
				}

				if err := b.session.TaskManager().UpdateProgress(progressArgs.TaskID, progressArgs.Progress); err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Error updating progress: %v", err)},
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
							{"type": "text", "text": fmt.Sprintf("Task %s progress updated: %s", progressArgs.TaskID, progressArgs.Progress)},
						},
					},
				})
			}

			if callParams.Name == "agent_peer_task_complete" {
				var completeArgs struct {
					TaskID    string   `json:"task_id"`
					Result    string   `json:"result"`
					Artifacts []string `json:"artifacts,omitempty"`
				}

				_ = json.Unmarshal(callParams.Arguments, &completeArgs)

				if completeArgs.TaskID == "" || completeArgs.Result == "" {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": "task_id and result are required values"},
							},
						},
					})
					break
				}

				if err := b.session.TaskManager().Complete(completeArgs.TaskID, completeArgs.Result, completeArgs.Artifacts); err != nil {
					b.writeResponse(StdioResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result: map[string]any{
							"isError": true,
							"content": []map[string]any{
								{"type": "text", "text": fmt.Sprintf("Error completing task: %v", err)},
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
							{"type": "text", "text": fmt.Sprintf("Task %s successfully marked as completed", completeArgs.TaskID)},
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
