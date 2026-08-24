# Changelog

All notable changes to this project are documented here. The format follows Keep a Changelog, and released versions use Semantic Versioning.

## [Unreleased]

## [0.1.2] - 2026-08-25

### Changed

- Update `github.com/miekg/dns` to 1.1.73, `golang.org/x/net` to 0.58.0, and `modernc.org/sqlite` to 1.57.0
- Build production container images with Go 1.27.0

## [0.1.1] - 2026-08-14

### Changed

- Update the pinned GitHub Actions used for repository checkout and Go setup

### Security

- Update the build toolchain to Go 1.26.6 for standard-library fixes affecting URL parsing, TLS post-handshake messages, ASN.1 decoding, and IDNA/Punycode hostname validation
- Update `modernc.org/sqlite` to 1.56.0 for upstream journal rollback integrity and `modernc.org/libc` deadlock fixes

## [0.1.0] - 2026-07-23

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
- Production Compose now passes documented runtime and notification settings from the deployment environment
- GHCR and Dokploy examples now consistently use the `Simplarix/dropwatch` repository and package

[Unreleased]: https://github.com/Simplarix/dropwatch/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/Simplarix/dropwatch/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/Simplarix/dropwatch/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/Simplarix/dropwatch/releases/tag/v0.1.0
