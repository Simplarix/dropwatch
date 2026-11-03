# Testing

## One-command verification

The canonical local and agent validation command is:

```bash
make verify
```

It runs, in order:

1. module checksum and tidy checks;
2. formatting checks;
3. `go vet ./...`;
4. `go test -race ./...`;
5. `go build ./...`.

CI uses the same target to prevent local/CI drift.

## Security verification

Run:

```bash
make security
```

This first runs the production Compose validation and then runs `govulncheck`, gosec, a production image build, an all-severity Trivy image scan, and a high/critical Trivy configuration scan. It downloads current vulnerability databases and therefore requires internet access. GitHub runs the same classes of checks on pull requests, on `main`, and weekly.

Trivy runs from an official release binary instead of compiling its large Go
dependency tree on every runner. `scripts/trivy.sh` pins version 0.72.0 and the
SHA-256 archive checksum for Linux/macOS on amd64/arm64. It verifies the download
before extraction and reuses the binary in the checkout's ignored `.cache/tools`
directory. Updating Trivy requires updating both the version and all four checksums.
Set `DROPWATCH_TOOLS_DIR` to use a different local tool directory, or `TRIVY` to
override the command used by Make. Vulnerability databases still update normally;
scan severities and failure behavior are unchanged.

## Actions runtime and storage

This public repository uses standard GitHub-hosted runners, which do not consume
the account's included paid compute allowance. A self-hosted runner is not needed
to avoid the private-repository minutes quota. See [GitHub Actions billing](https://docs.github.com/en/billing/concepts/product-billing/github-actions).

CI and Security cancel superseded runs for the same branch or pull request. Their
jobs have ten-minute timeouts; the tag-only release has a thirty-minute timeout
and a running release is not canceled by newer releases. All source, dependency, image, and
configuration checks remain enabled for pull requests and `main`.

The workflows do not upload Actions artifacts. Release binaries and checksums are
GitHub Release assets, and container images are published to GHCR. GitHub Free's
500 MB artifact allowance is separate from the included 10 GB of Actions cache
storage per repository. Only the existing `setup-go` caches are persisted between
runs; Trivy tools/databases and Docker layers do not create additional Actions
caches. The container-security job needs no host Go setup or cache now that Trivy
is downloaded as a binary. Avoid adding large artifact uploads or build caches
without considering their storage and retention.

## Production Compose validation

Run:

```bash
make compose-check
```

The check renders [compose.production.yml](../compose.production.yml) with
synthetic, non-secret runtime values and verifies that:

- the immutable `ghcr.io/simplarix/dropwatch@sha256:...` image is retained;
- ordinary runtime overrides such as timezone and concurrency reach the service;
- Discord and SMTP values are explicitly passed into the container;
- Docker Compose accepts the hardened production definition.

CI and the release security gate run this check so a syntactically valid Compose
change cannot silently discard deployment variables.

## Deterministic smoke test

Run:

```bash
make smoke
```

The script builds a temporary binary and uses a temporary SQLite database. It verifies:

- database initialization and migrations;
- watcher lease acquisition and graceful shutdown;
- a fresh scheduler heartbeat;
- `dropwatch health`;
- IDN and Punycode normalization;
- domain insertion without a live check;
- machine-readable list output.

The script does not use live RDAP or DNS and cleans up its temporary directory.

## Optional live-network smoke test

Run:

```bash
make smoke-live
```

After the deterministic checks, this command adds `example.com` with the normal initial check. It therefore exercises:

- IANA bootstrap download or cache reuse;
- real RDAP endpoint discovery and parsing;
- real DNS queries;
- check persistence and scheduling.

This is intentionally not part of CI. External availability, rate limits, and registry behavior must not make the deterministic suite flaky.

## Focused tests

Examples:

```bash
go test ./internal/domain
go test ./internal/rdap
go test -run TestAvailabilityRequiresSeparatedConfirmations ./internal/app
go test -run TestWatcherChecksDueDomainAndReleasesLock ./internal/scheduler
go test -run TestDiscordSend ./internal/notification
```

Use `-count=1` when investigating cache-sensitive failures and `-race` for concurrency changes.

## Test ownership

| Change area | Minimum validation |
| --- | --- |
| Domain normalization or states | `internal/domain` and `internal/app` tests |
| RDAP parsing or HTTP behavior | fixtures and `internal/rdap` tests |
| DNS behavior | local fake server or checker tests; no public DNS dependency |
| SQLite or migrations | `internal/storage` tests with a temporary database |
| Scheduler or concurrency | scheduler tests plus full race suite |
| Discord or SMTP | local protocol server or `httptest`; inspect secret-safe errors |
| CLI output | `internal/cli` tests and deterministic smoke test |
| TUI behavior | model/update tests when logic changes; manual terminal check for layout |
| Docker/runtime | `make docker-build`, `make compose-check`, container version command, and image healthcheck |
| Workflow YAML | `make compose-check` plus careful review of tag/deploy gates |

## Test design rules

- Use UTC and fixed clocks when time affects state transitions.
- Use seeded randomness or disable jitter in interval tests.
- Use `t.TempDir()` for databases and caches.
- Use local fixtures and bounded local HTTP/SMTP/DNS servers.
- Assert the negative safety rules, not only successful paths.
- Never place a real webhook, password, API key, or private domain in a fixture.
- Test cancellation paths so shutdown does not persist misleading failures.
