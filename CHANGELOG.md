# Changelog

All notable changes to this project are documented here. The format follows Keep a Changelog, and released versions use Semantic Versioning.

## [Unreleased]

## [0.1.12] - 2026-10-26

### Changed

- Update `github.com/charmbracelet/x/ansi` to 0.11.9 and its terminal rendering dependencies
- Update `modernc.org/sqlite` to 1.61.0 (SQLite 3.54.1) and its required `modernc.org/libc` dependency to 1.79.0
- Refresh the pinned Go 1.27.3 Alpine build image digest

## [0.1.11] - 2026-10-23

### Fixed

- Align dashboard headers and rows by visible terminal width, preserving columns with colored states and wide Unicode domain names

### Changed

- Replace the README's generated dashboard preview with a screenshot of the running TUI

## [0.1.10] - 2026-10-22

### Added

- MIT license, included in release downloads and container images
- Public installation instructions, a dashboard preview with sample data, and Simplarix maintainer attribution

### Changed

- Prepare the existing project for its public open-source release, preserving version history
- Update the deployment guide for public repository and container access
- Point CODEOWNERS to the current maintainer account

### Fixed

- Reserve hosted-runner disk space for the container security scan's vulnerability database

## [0.1.9] - 2026-10-21

### Changed

- Update `golang.org/x/net` to 0.61.0, `golang.org/x/sys` to 0.49.0, and `golang.org/x/text` to 0.43.0

### Security

- Require Go 1.26.10 for local builds, CI, and release binaries, and build production container images with Go 1.27.3 to distrust compromised intermediate certificates in `crypto/x509` ([GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809))

## [0.1.8] - 2026-10-12

### Changed

- Update `golang.org/x/net` to 0.60.0

### Security

- Require Go 1.26.9 for local builds, CI, and release binaries, and build production container images with Go 1.27.2 to include the October standard-library security fixes
- Pick up upstream HTTP/2 fixes in `golang.org/x/net` for header validation and limits, flow control, excessive CPU consumption, and an HPACK encoder race

## [0.1.7] - 2026-10-06

Version 0.1.6 was tagged but not published because the release runner ran out of disk space while downloading the vulnerability database.

### Changed

- Update `modernc.org/sqlite` to 1.60.1 and its required `modernc.org/libc` dependency to 1.77.1

### Fixed

- Pick up SQLite driver fixes for WAL shared-memory read faults and statement parameter binding performance
- Free space occupied by unused SDKs on the GitHub-hosted release runner before security scans and image builds

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

[Unreleased]: https://github.com/Simplarix/dropwatch/compare/v0.1.12...HEAD
[0.1.12]: https://github.com/Simplarix/dropwatch/compare/v0.1.11...v0.1.12
[0.1.11]: https://github.com/Simplarix/dropwatch/compare/v0.1.10...v0.1.11
[0.1.10]: https://github.com/Simplarix/dropwatch/compare/v0.1.9...v0.1.10
[0.1.9]: https://github.com/Simplarix/dropwatch/compare/v0.1.8...v0.1.9
[0.1.8]: https://github.com/Simplarix/dropwatch/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/Simplarix/dropwatch/compare/v0.1.5...v0.1.7
[0.1.5]: https://github.com/Simplarix/dropwatch/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/Simplarix/dropwatch/compare/v0.1.2...v0.1.4
[0.1.2]: https://github.com/Simplarix/dropwatch/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/Simplarix/dropwatch/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/Simplarix/dropwatch/releases/tag/v0.1.0
