# DropWatch

DropWatch is a self-hosted Go application for monitoring registered domains. It uses RDAP as its primary source, treats DNS as a supporting signal, records relevant changes in SQLite, and sends notifications through Discord or SMTP. A single binary provides a scriptable CLI, an interactive terminal dashboard, and a long-running watcher.

## Features

- Unicode-domain and Punycode normalization through IDNA
- IANA RDAP bootstrap discovery with local caching and last-known-good fallback
- RDAP status, registrar, expiration-date, and nameserver tracking
- supplementary NS, SOA, A, AAAA, and MX lookups
- conservative availability decisions with multiple time-separated confirmations
- adaptive intervals, jitter, exponential backoff, and `Retry-After` support
- SQLite with WAL, embedded migrations, foreign keys, and durable history
- deduplicated Discord and SMTP delivery with attempt history
- exclusive watcher lease, heartbeat, bounded concurrency, and graceful shutdown
- Bubble Tea terminal dashboard
- small hardened multi-architecture container and tag-gated GHCR release workflow

## Local development and testing

Prerequisites:

- Go 1.26.5 or newer
- Docker only for container tests
- a C compiler for `go test -race` on platforms where the Go race detector requires it

The fastest deterministic validation is:

```bash
make verify
make smoke
```

`make verify` checks formatting, runs `go vet`, executes the full test suite with the race detector, and builds the application. `make smoke` exercises initialization, the watcher lease and heartbeat, health checking, IDN/Punycode handling, and JSON CLI output against an isolated temporary database. It does not need internet access.

An optional real-network smoke test is available:

```bash
make smoke-live
```

It performs the deterministic smoke test first and then checks `example.com` through the real IANA bootstrap, RDAP, and DNS path. Use it only when outbound network access is expected.

For a quicker edit/test loop:

```bash
make test
go test ./internal/domain
go test -run TestAvailabilityRequiresSeparatedConfirmations ./internal/app
make build
./dropwatch version
```

More detail is available in [docs/testing.md](docs/testing.md).

## Installation

From a checkout:

```bash
make build
./dropwatch version
```

Installation through Go is:

```bash
go install github.com/Simplarix/dropwatch/cmd/dropwatch@latest
```

## Quick start

```bash
dropwatch init --write-config
dropwatch domain add example.com
dropwatch domain add https://bücher.de/some/path
dropwatch domain list
dropwatch domain show example.com
dropwatch check example.com
dropwatch history example.com --limit 100
dropwatch dashboard
```

Machine-readable commands support `--json`; automation can use `--quiet`. A network or registry failure makes `check` return a non-zero exit status, but it never marks a domain as available.

The production process is:

```bash
dropwatch watch
```

`SIGINT` and `SIGTERM` trigger a graceful shutdown. Only one watcher may own a database at a time.

## CLI

```text
dropwatch init [--write-config]
dropwatch domain add DOMAIN [--no-check]
dropwatch domain remove DOMAIN
dropwatch domain list [--json]
dropwatch domain show DOMAIN [--json]
dropwatch check DOMAIN [--json]
dropwatch check --all [--json]
dropwatch history DOMAIN [--limit N] [--json]
dropwatch watch
dropwatch dashboard
dropwatch notify test
dropwatch health [--json]
dropwatch version [--json]
```

Global flags such as `--database`, `--config`, `--timezone`, `--max-concurrency`, `--http-timeout`, `--log-level`, and `--log-format` override configuration values.

Dashboard keys:

```text
↑/↓ or j/k   Navigate
Enter        Open details
r            Check the selected domain
R            Check every domain
a            Add a domain
d            Remove with confirmation
h            Show history
?            Show help
q            Quit
```

## Configuration

Configuration precedence is:

```text
CLI flag > DROPWATCH_ environment variable > YAML > default
```

See [config.example.yaml](config.example.yaml) for a complete YAML example and [.env.example](.env.example) for container variables. Supported environment variables include:

- `DROPWATCH_DATABASE_PATH`
- `DROPWATCH_CONFIG_PATH`
- `DROPWATCH_LOG_LEVEL`
- `DROPWATCH_LOG_FORMAT`
- `DROPWATCH_TIMEZONE`
- `DROPWATCH_MAX_CONCURRENCY`
- `DROPWATCH_HTTP_TIMEOUT`
- `DROPWATCH_DISCORD_WEBHOOK_URL`
- `DROPWATCH_SMTP_HOST`, `DROPWATCH_SMTP_PORT`
- `DROPWATCH_SMTP_USERNAME`, `DROPWATCH_SMTP_PASSWORD`
- `DROPWATCH_SMTP_FROM`, `DROPWATCH_SMTP_TO`
- `DROPWATCH_SMTP_REQUIRE_TLS`

All timestamps are stored in UTC. `timezone` only changes human-readable output. Credentials belong in an untracked environment or the deployment platform's secret store. Remote RDAP bootstrap and service URLs must use public HTTPS endpoints. SMTP requires STARTTLS except for explicit loopback-only test servers.

## Availability and safety model

RDAP is the primary source. DNS failures are supporting information and never prove availability. An authoritative RDAP 404 first produces `possibly_available`. By default, three confirmations separated by at least one minute are required before the state becomes `available`. Any registration signal wins when providers disagree.

`clientHold`, missing DNS records, timeouts, HTTP 429, and HTTP 5xx explicitly do not mean that a domain is available. Provider extension guidance is in [docs/providers.md](docs/providers.md).

DropWatch does not guarantee domain registration. RDAP and DNS results are not a legal or technical commitment from a registrar. Registry policy, reserved or premium names, registrar-specific restrictions, and state changes between observation and registration can all affect the outcome.

## Docker

Build and run locally:

```bash
docker build -t dropwatch:local .
docker volume create dropwatch-data
docker run --rm \
  --name dropwatch \
  -v dropwatch-data:/data \
  -e DROPWATCH_DATABASE_PATH=/data/dropwatch.db \
  dropwatch:local
```

Or use Compose:

```bash
export DROPWATCH_IMAGE='ghcr.io/nbauditz/dropwatch@sha256:REPLACE_WITH_RELEASE_DIGEST'
docker compose -f compose.production.yml up -d
```

Use the exact digest from the release's `image-digest.txt`; floating tags such as `latest` are intentionally unsupported for production. [compose.production.yml](compose.production.yml) uses a read-only root filesystem, drops every Linux capability, enables `no-new-privileges`, limits resources and logs, and exposes no port. Its image healthcheck calls `dropwatch health --quiet`.

The `/data` volume contains SQLite data and the RDAP bootstrap cache. Back it up using a SQLite-aware snapshot or while the container is stopped; do not copy individual WAL files independently.

## Architecture and agent-assisted development

- [docs/architecture.md](docs/architecture.md) explains package boundaries and data flow.
- [docs/testing.md](docs/testing.md) describes deterministic and live validation.
- [docs/agentic-development.md](docs/agentic-development.md) explains how Codex, Claude Code, and GitHub Copilot are configured.
- [AGENTS.md](AGENTS.md) is the repository-wide source of truth for coding agents.
- [REVIEW.md](REVIEW.md) contains the security and correctness review checklist.

## Security checks, release, and Dokploy

Run `make security` for reachable Go vulnerability checks, static security analysis, repository configuration scanning, and a full image CVE scan. GitHub repeats these checks for pull requests, `main`, and weekly schedules; Dependabot monitors Go modules, Actions, and Docker references.

A normal push to `main` runs CI only. Only a newly pushed stable tag in the exact form `vX.Y.Z`, whose commit belongs to `main`, creates the GitHub Release and multi-architecture image with provenance and an SBOM. Deployment remains deliberately private and manual; the GitHub-hosted runner never contacts Dokploy. See [docs/dokploy.md](docs/dokploy.md).
