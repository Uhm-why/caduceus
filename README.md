# Caduceus: Agent-to-Agent mTLS 1.3 MCP Server

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/Uhm-why/caduceus?style=flat-square&logo=go&logoColor=white)
![GitHub License](https://img.shields.io/github/license/Uhm-why/caduceus)
![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Uhm-why/caduceus/ci.yml)
---
Caduceus is a bidirectional Agent-to-Agent mTLS 1.3 MCP Server that allows one agent to delegate work to another agent on the same machine, across the local network or even across the internet. Updates are provided via each agent's Maildir inbox.

Once work has been delegated, the agent that requested the work be performed can query the running agent for status updates.

# Core Capabilities
- Bidirectional mTLS 1.3 Transport (Curve25519 / Ed25519)
- Cross-Platform Maildir Durability (tmp/, /new, /cur)
- Native Context Compactor (ANSI stripper, RLE line collapsing, spillover windowing with TTL cleanup)
- Zero-touch Node Initialization

# Quick Start
1. Build the static binary:
`CGO_ENABLED=0 go build ./cmd/caduceus`
2. Bootstrap credentials:
`./caduceus init`
3. Start Caduceus as a listener or connect to a peer:
```bash
#Run as a listener
./caduceus -listen [IP:Port]

#Connect to peer
./caduceus -ip [IP:Port]
```
You can run `./caduceus -h` for other configuration options.

The order of precedence for configuration is:
`CLI Flags > config.json > Hardcoded Defaults (config.json is stored by default at ~/.config/caduceus/config.json with ~/.caduceus/config.json as a fallback)`

# Built-in MCP Tools
The following tools are provided by the MCP server:
- Direct Messaging:
  - `agent_peer_send` allows one agent to send a message to another agent's inbox.
  - `agent_peer_inbox` is used to search for new work that the agent has received.
- Consultative Review:
  - `agent_peer_consult` allows an agent to consult another agent for a second opinion.
  - `agent_peer_review` is a specific request for another agent to review work accomplished (code, writing, etc.).
  - `agent_peer_reply` allows the agent receiving a consultation request to send a message to the requesting agent's inbox.
- Task Delegation:
  - `agent_peer_delegate` is used when the current agent lacks the time or resources to perform a task. It is completely handed off to the receiving agent while the requesting agent moves on to other work.
  - `agent_peer_task_status` allows an agent to query the status of delegated work.
  - `agent_peer_task_cancel` sends a request to the agent currently performing delegated work to cancel the task.
  - `agent_peer_task_complete` marks the task as complete and sends a message to the requesting agent's inbox with the result.
  - `agent_peer_task_progress` updates the progress in the task tracker for when the delegating agent requests an update.
- Discovery / Inspection:
  - `agent_peer_status` checks the current connection state to the configured peer.
  - `agent_peer_capabilities` queries and returns the peer agent's supported feature set.

# Multi-Node Trust Setup
When pairing two machines (e.g. a laptop and a Raspberry Pi):
1. Run `./caduceus init` on each node.
2. Append each node's ca.pem to the other's ~/.config/caduceus/ca.pem .

Both nodes will now mutually authenticate over mTLS 1.3.

# Roadmap
- Greylist Execution Security: Out-of-band one-time verification tokens for sensitive/elevated tool execution.
- Automated Trust Management: `caduceus trust add` CLI helper for federating node CAs without manual file edits.
- Stream Compression: Optional wire-level stream compression over mTLS framing.
