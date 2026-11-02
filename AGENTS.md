# Caduceus: Agent Development Guidelines

## Project Philosophy
- **Zero External Dependencies:** Built 100% with the Go standard library. Do not introduce third-party modules to `go.mod`.
- **Zero-Dependency Static Binary:** Must compile cleanly with `CGO_ENABLED=0 go build ./cmd/caduceus`.
- **Peer-to-Peer Symmetry:** Every Caduceus node is functionally identical (possesses CA, leaf cert, key, mailbox, and spillover); runtime flags determine whether a node listens (`-listen`) or connects (`-ip`).
- **Clean-Room Code Policy:** The repository author writes the code. Agents must focus on architecture, logic flow, mechanical breakdowns, and review unless explicitly instructed to write code or test suites.

## Verification & Quality Gates
Before declaring any task or PR complete, all quality gates must pass locally:
1. `go test -count=1 -race ./...` (all tests passing across all packages)
2. `go vet ./...` (zero warnings or errors)
3. `gofmt -l .` (zero unformatted files)

## Cross-Platform Constraints
- Code must support Linux (`aarch64`, `x86_64`), macOS, and Windows.
- Windows filesystems (NTFS/FAT32) do not implement POSIX permission bitmasks (`0600`, `0644`). Any test verifying exact octal file modes must be guarded with `if runtime.GOOS != "windows"`.
- All path operations must use `path/filepath` (not raw string concatenation or `/`) to preserve Windows backslash handling.

## Architecture & Subsystems
- `cmd/caduceus/`: CLI entrypoint, flag parsing, and subcommand dispatch (`init`).
- `internal/config/`: Configuration precedence (`Flags > config.json > Hardcoded Defaults`), invariant validation in `Validate()`, and zero-touch `caduceus init` bootstrap.
- `internal/mailbox/`: Cross-platform Maildir spool (`tmp/`, `new/`, `cur/`) with atomic rename promotion and fsync durability.
- `internal/compactor/`: Deterministic context reduction pipeline (ANSI stripping, run-length line deduping, head-tail spillover windowing, TTL age pruning, and idempotent deletion).
- `internal/peer/`: Wire framing, bidirectional mTLS 1.3 state machine, capability handshake, consultative tools, task delegation lifecycle, and stdio MCP bridge.
