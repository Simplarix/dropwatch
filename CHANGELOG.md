# Changelog

All notable changes to this project are documented here. The format follows Keep a Changelog, and released versions use Semantic Versioning.

## [Unreleased]

### Added

- Initial CLI, Bubble Tea dashboard, and production watcher
- Conservative RDAP/DNS state model with availability confirmations
- SQLite migrations, event history, scheduler lease, and heartbeat
- Discord and SMTP notifications with deduplication and retries
- Docker, GHCR, GitHub Release, and Dokploy automation
- Repository guidance for Codex, Claude Code, and GitHub Copilot
- Deterministic local smoke and verification workflows
- Public-HTTPS-only RDAP networking with redirect and private-address SSRF protection
- Automated dependency, source, configuration, and container vulnerability scanning
- Digest-only hardened production Compose deployment and private Dokploy workflow

### Fixed

- Concurrent watcher and healthcheck startup no longer races while enabling SQLite WAL mode
- Discord transport errors no longer risk persisting or logging webhook credentials
- Invalid timing, jitter, body-size, scheduler, and plaintext SMTP settings now fail closed
