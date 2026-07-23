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

This runs `govulncheck`, gosec, a production image build, an all-severity Trivy image scan, and a high/critical Trivy configuration scan. It downloads current vulnerability databases and therefore requires internet access. GitHub runs the same classes of checks on pull requests, on `main`, and weekly.

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
| Docker/runtime | `make docker-build`, container version command, and image healthcheck |
| Workflow YAML | YAML/Compose validation and careful review of tag/deploy gates |

## Test design rules

- Use UTC and fixed clocks when time affects state transitions.
- Use seeded randomness or disable jitter in interval tests.
- Use `t.TempDir()` for databases and caches.
- Use local fixtures and bounded local HTTP/SMTP/DNS servers.
- Assert the negative safety rules, not only successful paths.
- Never place a real webhook, password, API key, or private domain in a fixture.
- Test cancellation paths so shutdown does not persist misleading failures.
