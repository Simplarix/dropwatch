# DropWatch Agent Guide

## Purpose

This file is the repository-wide source of truth for coding agents. Keep it concise and practical. Read the linked documents only when the task touches their subject.

## Start here

- Read `README.md` for product behavior and commands.
- Read `docs/architecture.md` before changing state decisions, storage, scheduling, notifications, or package boundaries.
- Read `docs/testing.md` before adding tests or changing validation.
- Use `REVIEW.md` for the final self-review.
- Preserve unrelated working-tree changes.

## Canonical commands

```bash
make verify       # format check, vet, race tests, build
make smoke        # deterministic CLI/watcher/health smoke test
make smoke-live   # optional real IANA/RDAP/DNS test
make docker-build # local production-image build
make compose-check # production Compose syntax and variable propagation
make security     # source, dependency, config, and image security scans
```

During iteration, run focused package tests. Before handoff, run `make verify`. Run `make smoke` after CLI, config, storage, scheduler, or health changes. Live-network tests are optional and never replace deterministic tests.

## Product invariants

- RDAP is the primary availability source; DNS is supplementary only.
- Never map timeout, cancellation, HTTP 429, HTTP 5xx, malformed data, or missing DNS records to `available`.
- `clientHold` and `serverHold` are registered states.
- An authoritative RDAP 404 produces `possibly_available`, not a registration guarantee.
- `available` requires the configured number of time-separated confirmations.
- Any credible registration signal wins over an availability signal.
- Store internal timestamps in UTC; apply configured timezones only at output boundaries.
- Keep external requests context-bound, timeout-limited, redirect-limited, and response-size-limited.
- Never expose secrets, webhook URLs, SMTP passwords, or raw external error bodies.

## Architecture boundaries

- `internal/domain`: pure domain types, normalization, and state mapping.
- `internal/app`: orchestration and conservative decision logic.
- `internal/storage`: repository contracts and SQLite implementation.
- `internal/rdap`, `internal/dnscheck`, `internal/availability`: external observations.
- `internal/scheduler`: intervals, backoff, lease, heartbeat, and concurrency.
- `internal/notification`: outbox, retries, Discord, and SMTP.
- `internal/cli` and `internal/tui`: presentation only; do not duplicate business decisions here.
- `migrations`: embedded, numbered, forward-only schema changes.

Business logic must depend on repository/provider/notifier interfaces, not SQL or transport details.

## Editing rules

- Make the smallest coherent change that fully solves the task.
- Search with `rg` and inspect interfaces plus tests before implementation.
- Do not rewrite an applied migration; add the next numbered migration.
- Keep JSON field names and CLI exit behavior backward-compatible unless the task explicitly introduces a breaking change.
- Keep example secrets empty.
- Do not add live-network dependencies to unit or CI tests.
- Use fixed clocks, seeded randomness, temporary directories, fixtures, and local protocol servers.
- Update README, examples, and docs when configuration or user-visible behavior changes.
- Do not commit binaries, SQLite files, caches, `.env`, or generated coverage output.
- Do not push, tag, publish images, or deploy unless the user explicitly authorizes it.

## Change-specific validation

- State or availability logic: domain and app transition tests, including negative safety cases.
- RDAP: parser fixtures plus HTTP timeout/status/body-limit tests.
- DNS: local fake server or deterministic checker tests.
- Storage/migrations: temporary SQLite repository and migration tests.
- Scheduler/concurrency: scheduler tests plus `go test -race ./...`.
- Notifications: local HTTP/SMTP tests, attempt persistence, deduplication, and secret-safe failures.
- CLI: command-output tests and `make smoke`.
- TUI: update-model tests where practical and a manual layout check.
- Docker/workflows: image build, version command, user/health metadata, YAML/Compose validation, and `make security`.
- Compose/environment changes: `make compose-check` with synthetic non-secret values.

## Code Review Rules

Flag changes that:

- make availability less conservative or let DNS decide it;
- lose checks, events, confirmations, notification attempts, or scheduler state across restart;
- allow two watchers or duplicate checks for one domain;
- bypass notification deduplication or persist secrets;
- follow unbounded redirects, read unbounded bodies, or omit contexts/timeouts;
- change an existing migration instead of adding one;
- break stable JSON fields, exit codes, graceful shutdown, non-root runtime, digest-only production deployment, or tag-only releases;
- add flaky live-network tests to the deterministic suite.

## Definition of done

- Behavior is implemented without central TODOs or placeholders other than user-specific deployment values.
- Relevant tests cover success, failure, and safety paths.
- `make verify` passes.
- `make smoke` passes when its covered areas changed.
- `make security` passes before a production release.
- Documentation and examples match behavior.
- The final handoff reports changed files, validation, assumptions, and any user action.
