---
name: caduceus
description: Delegate tasks or consult peer agents over mTLS 1.3 MCP.
version: 0.3.0
author: Dustin Hacker (Uhm-why)
license: Apache-2.0
platforms: [linux, macos, windows]
metadata:
  hermes:
    tags: [p2p, mtls, mcp, delegation]
---

# Caduceus Skill

Bidirectional peer-to-peer mTLS 1.3 JSON-RPC MCP server for inter-agent delegation, consultative review, and durable message spooling.

## When to Use

- When delegating CPU-heavy, long-running, or independent tasks to a secondary agent or dedicated compute node.
- When requesting a second opinion or independent review of code, diffs, writing, or plans from a peer agent.
- When exchanging asynchronous, durable messages or work packages across systems via the Maildir spool.

Don't use for:
- Local single-agent operations that do not involve another node or agent instance.
- Short, synchronous tasks that can run directly within the current turn.

## Prerequisites

- Go v1.27.1+ (or host Go toolchain) if compiling from source.
- Network connectivity between nodes on the configured peer port (default `:8443`).
- Initialized cryptographic identity and mailbox spool on each node.

## Building & Initialization

### Building the Static Binary
Run:
`CGO_ENABLED=0 go build -o caduceus ./cmd/caduceus`

Git repository: https://github.com/Uhm-why/caduceus

### Initialization
Run:
`caduceus init [optional-target-dir]`

Default config directory: `~/.config/caduceus` (fallback: `~/.caduceus`)

Initialization provisions:
- Certificates & Keys:
  - `ca.key` (0600): Root Ed25519 CA private key.
  - `ca.pem` (0644): Root CA certificate (10-year validity).
  - `key.pem` (0600): Node leaf private key.
  - `cert.pem` (0644): Node leaf certificate (1-year validity; SANs: `caduceus-agent`, `localhost`, `127.0.0.1`, `::1`).
- Mailbox Spool: `mailbox/` (`tmp/`, `new/`, `cur/`) and `spillover/`.
- Configuration: `config.json` with default listen address `:8443` and peer SAN `caduceus-agent`.

Note on cross-node trust: Both nodes must trust each other's CA certificates. Copy or bundle root CAs to verify peer identities.

## Running & MCP Configuration

Caduceus serves as an MCP stdio bridge on stdin/stdout while simultaneously maintaining the peer mTLS 1.3 network transport.

Configuration precedence:
CLI Flags > config.json > Hardcoded Defaults

### Running Modes
- Listener Mode (waits for inbound peer connections):
  `caduceus -listen :8443`
- Dial Mode (connects out to a remote peer listener):
  `caduceus -ip 192.168.68.57:8443 -san caduceus-agent`

### Hermes MCP Integration
To expose Caduceus tools directly in Hermes Agent, configure it as a local MCP server in `~/.hermes/config.yaml`:

```yaml
mcp_servers:
  caduceus:
    command: "/path/to/caduceus"
    args: ["-listen", ":8443"]
```

Or when dialing an existing remote peer:

```yaml
mcp_servers:
  caduceus:
    command: "/path/to/caduceus"
    args: ["-ip", "192.168.68.57:8443", "-san", "caduceus-agent"]
```

## CLI Flags

- `-config string`: Directory or file path where Caduceus configuration is stored.
- `-listen string`: Address to bind for incoming peer connections (e.g. `:8443`).
- `-ip string`: Address `[host:port]` of the remote peer to connect to.
- `-san string`: Expected Subject Alternative Name (SAN) of the remote peer (default: `caduceus-agent`).
- `-mailbox-path string`: Directory where Caduceus persists queued Maildir messages.
- `-dial-timeout string`: Duration to spend attempting to dial a peer before aborting (e.g. `10s`).
- `-handshake-timeout string`: Seconds to wait on mTLS handshake before aborting.
- `-idle-timeout string`: Maximum idle connection time before aborting.
- `-keepalive string`: Keepalive probe interval.
- `-max-frame-size int`: Maximum wire frame size in bytes.

## Tools Reference & Lifecycle

Caduceus exposes 12 MCP tools categorized by capability:

### 1. Discovery & Inspection
- `agent_peer_status`: Inspect current mTLS connection status and remote peer metadata.
- `agent_peer_capabilities`: Query the remote peer's advertised feature set and tool availability.

### 2. Consultative Review
- `agent_peer_consult`: Request an opinion or architectural guidance from the peer.
- `agent_peer_review`: Submit completed work (code diffs, drafts, plans) for formal review.
- `agent_peer_reply`: Send a consultation response or review verdict back to the requesting agent.

### 3. Task Delegation Lifecycle
- `agent_peer_delegate`: Dispatch a heavy or independent task payload to the remote peer.
- `agent_peer_task_status`: Query execution state of a previously delegated task.
- `agent_peer_task_progress`: Update progress notes or percentage on an assigned task.
- `agent_peer_task_complete`: Mark a delegated task complete and return the final output payload.
- `agent_peer_task_cancel`: Request termination of a delegated task currently in flight.

### 4. Direct Messaging & Mailbox
- `agent_peer_send`: Deposit a durable message into the peer's inbox spool.
- `agent_peer_inbox`: Inspect and poll local inbox for queued messages and delegated work.

## Multi-Node Trust Management

Caduceus nodes mutually authenticate via mTLS 1.3 using a shared trust bundle. Use the `trust` subcommand to manage trusted CA certificates:
- `caduceus trust list [-ca path] [-key path]`: Print subject CN, SHA-256 fingerprint, and local-root status for all trusted certificates.
- `caduceus trust add [-ca path] <cert.pem>`: Validate and atomically append a peer's CA certificate to the local trust bundle.
- `caduceus trust remove [-ca path] [-key path] <fingerprint>`: Atomically remove a trusted certificate by its SHA-256 fingerprint (refuses to remove the local node's own root CA).

## Greylist Execution Security & Authorization

Caduceus gates every inbound `agent/task/dispatch` request through a policy-based authorization engine before execution. Each request is evaluated against standing policy rules and resolves to one of three decisions:
- **Allow**: Executes immediately.
- **Greylist** *(default)*: Halts execution and registers a pending authorization. An out-of-band one-time password (OTP) is generated and surfaced only on the receiving node's local console/log (never written to the mailbox, to prevent token leakage to local file readers). After 3 incorrect verification attempts, the pending request is automatically denied.
- **Deny**: Immediately rejected.

Use the `auth` subcommand to manage standing policy rules and inspect pending challenges:
- `caduceus auth rules [-policy path]`: Display the default action and all configured rules in priority order.
- `caduceus auth allow [-policy path] <pattern> [description]`: Prepend an allow rule matching a glob pattern (e.g. `terminal.status`).
- `caduceus auth deny [-policy path] <pattern> [description]`: Prepend a deny rule matching a glob pattern (e.g. `system.reboot`).
- `caduceus auth remove [-policy path] <pattern>`: Remove a rule by its exact pattern.
- `caduceus auth pending [-mailbox path]`: List active authorization challenges awaiting operator verification.

Policy rules are persisted atomically to `policy.json` (default: `~/.config/caduceus/policy.json`) and are evaluated with "first-match wins" precedence; newly added rules are prepended so the most recent operator decision always takes priority over broader wildcard rules.
