# Changelog

All notable changes to this project are documented here. The format follows Keep a Changelog, and released versions use Semantic Versioning.

## [Unreleased]

## [0.1.5] - 2026-10-01

### Changed

- Update `golang.org/x/net` to 0.59.0, `golang.org/x/sys` to 0.48.0, and `golang.org/x/text` to 0.42.0
- Update `modernc.org/sqlite` to 1.59.0 and `modernc.org/libc` to 1.75.7
- Refresh the pinned Go 1.27.1 Alpine build image digest

## [0.1.4] - 2026-09-07

Version 0.1.3 was tagged but not published because release validation detected a watcher shutdown race.

### Changed

- Update `modernc.org/sqlite` to 1.58.0 (SQLite 3.53.4), `modernc.org/libc` to 1.75.6, and `modernc.org/memory` to 1.12.1
- Build production container images with Go 1.27.1

### Fixed

- Treat heartbeat cancellation during shutdown as graceful termination and wait for running checks before releasing the watcher lease
- Start the bounded lease-release context during cleanup so long-running watchers can release their lease on shutdown

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

[Unreleased]: https://github.com/Simplarix/dropwatch/compare/v0.1.5...HEAD
[0.1.5]: https://github.com/Simplarix/dropwatch/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/Simplarix/dropwatch/compare/v0.1.2...v0.1.4
[0.1.2]: https://github.com/Simplarix/dropwatch/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/Simplarix/dropwatch/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/Simplarix/dropwatch/releases/tag/v0.1.0
