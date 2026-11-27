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
- Automated Multi-Node Trust Management (`caduceus trust`)
- Greylist Execution Security & Policy Enforcement (`caduceus auth`)

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

# Running in a Container Sandbox
Caduceus provides a multi-stage `Dockerfile` creating an unprivileged, minimal Alpine-based container sandbox (~15MB total) for running agent nodes in isolated environments:

1. Build the container image:
```bash
docker build -t caduceus .
```

2. Bootstrap local node credentials in a persistent volume:
```bash
docker run --rm -v caduceus-data:/home/caduceus/.config/caduceus caduceus init
```

3. Run Caduceus as a listener:
```bash
docker run -d --name caduceus-node -p 8443:8443 -v caduceus-data:/home/caduceus/.config/caduceus caduceus -listen :8443
```

*(Note: `podman` can be substituted for `docker` directly with identical arguments).*

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
1. Run `./caduceus init` on each node to bootstrap local credentials.
2. Copy each node's public `ca.pem` to the other node.
3. Import the peer's root CA into the local trust bundle:
```bash
./caduceus trust add /path/to/peer-ca.pem
```
4. Verify trusted certificates and fingerprints:
```bash
./caduceus trust list
```

To revoke a peer's trust at any time without affecting your local root CA:
```bash
./caduceus trust remove <fingerprint>
```

Both nodes will now mutually authenticate over mTLS 1.3.

# Authorization & Greylist Policy Management
Caduceus implements a defense-in-depth authorization engine (`internal/auth`) to protect nodes against unauthorized task execution. Inbound tasks are evaluated against standing policy rules using a tri-state model:
- **Allow**: Executes immediately without interruption.
- **Greylist** *(Default)*: Halts execution, generates an out-of-band one-time verification token (OTP), notifies the operator, and holds the task pending authorization.
- **Deny**: Rejects the task dispatch immediately.

### Managing Policy Rules
View current policy rules and default action:
```bash
./caduceus auth rules
```

Allow or deny specific action patterns (shell-style globbing):
```bash
# Explicitly allow read-only status checks
./caduceus auth allow "terminal.status" "allow status queries"

# Explicitly deny destructive commands
./caduceus auth deny "system.reboot" "block reboot requests"
```

Rules use an override-prepend priority: newest operator rules always take immediate precedence over broader wildcards.

Remove an existing rule:
```bash
./caduceus auth remove "terminal.status"
```

Inspect active authorization challenges waiting for human verification:
```bash
./caduceus auth pending
```

# Roadmap
- Stream Compression: Optional wire-level stream compression over mTLS framing.
