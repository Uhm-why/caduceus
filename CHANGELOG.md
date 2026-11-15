# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Automated trust management (`caduceus trust add`)

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
