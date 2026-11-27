# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2026-11-27

### Added

- Greylist Execution Security engine (`internal/auth`) providing policy-based task authorization with a tri-state decision model (`allow`, `greylist`, `deny`).
- Thread-safe policy evaluation engine (`internal/auth/policy.go`) using shell-style glob pattern matching (`path.Match`) with override-prepend rule precedence (newest operator decision wins).
- Atomic `policy.json` persistence (`LoadPolicy`/`SavePolicy`) with human-readable JSON policy action serialization and safe fallback to default policy when unconfigured.
- Human-in-the-loop authorization manager (`internal/auth/manager.go`) implementing one-time password (OTP) challenge/response using `crypto/rand` and constant-time verification via `crypto/subtle`.
- Brute-force lockout: pending authorizations are automatically denied after 3 failed OTP verification attempts.
- Protocol-level enforcement in `internal/peer/session.go` gating `agent/task/dispatch` against the authorization policy, tied to the peer's mTLS-verified SAN identity.
- Out-of-band OTP delivery: challenge tokens are never written to the Maildir mailbox, and are instead surfaced only via the local operator console, preventing token leakage to local file readers.
- CLI operator subcommands (`caduceus auth rules`, `caduceus auth allow <pattern>`, `caduceus auth deny <pattern>`, `caduceus auth remove <pattern>`, `caduceus auth pending`) for managing standing policy rules and inspecting active authorization challenges.
- Comprehensive unit test suite for the authorization engine covering policy evaluation, override precedence, OTP lifecycle, concurrency stress testing under the Go race detector, and policy file persistence round-trips.

## [0.2.0] - 2026-11-20

### Added

- Automated Trust Management engine (`internal/trust`) supporting PEM CA bundle loading, X.509 validation, SHA-256 fingerprinting, duplicate detection, and atomic file commits.
- Local Root CA protection preventing accidental removal of the local node's root CA.
- CLI trust management subcommands (`caduceus trust list`, `caduceus trust add <cert.pem>`, and `caduceus trust remove <fingerprint>`).
- Comprehensive unit test suite for trust lifecycle management under concurrency race detection (`internal/trust/trust_test.go`).

### Changed

- Modernized filesystem error checking across the codebase from legacy `os.IsNotExist` to `errors.Is(err, os.ErrNotExist)`.

## [0.1.0] - 2026-11-02

### Added

- Bidirectional mTLS 1.3 transport using Ed25519 and Curve25519.
- Pure Go cryptographic node bootstrap (`caduceus init`) generating 10-year Root CA and 1-year leaf certificates.
- Cross-platform Maildir message spooling (`tmp/`, `new/`, `cur/`).
- Deterministic Context Compactor (ANSI stripping, RLE line collapsing, spillover windowing with TTL cleanup).
- Built-in MCP Server providing 10 tools across Direct Messaging, Consultative Review, Task Delegation, and Discovery.
- Multi-OS CI matrix across Ubuntu, macOS, and Windows with concurrency race detection.
- Supply chain automation with Dependabot, govulncheck, and golangci-lint v2.
- Repository governance files (CHANGELOG.md, SECURITY.md, CONTRIBUTING.md, CODE_OF_CONDUCT.md, PR and issue templates).
