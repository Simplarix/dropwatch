# Architecture

## System overview

DropWatch is a single Go binary with three operating modes:

1. one-shot CLI commands for automation;
2. an interactive Bubble Tea dashboard;
3. a long-running watcher used by Docker and Dokploy.

All modes share the same application service, repository contracts, and SQLite database.

```text
CLI / TUI / watcher
        |
        v
application checker
   |       |        |
   v       v        v
RDAP     DNS     availability providers
   \       |        /
    \      v       /
     state decision
          |
          v
SQLite transaction -> events -> notification outbox -> Discord / SMTP
```

## Non-negotiable domain invariant

Uncertainty must resolve to the safer state.

- DNS alone never determines availability.
- `clientHold` and `serverHold` still describe registered domains.
- Timeouts, cancellation, HTTP 429, HTTP 5xx, malformed responses, and missing DNS records never produce `available`.
- An authoritative RDAP 404 produces `possibly_available`.
- `available` requires the configured number of time-separated confirmations.
- Any credible registration result overrides an availability result.

Changes to `internal/app`, `internal/domain`, `internal/rdap`, or `internal/availability` must preserve this invariant and include transition tests.

## Package map

| Path | Responsibility |
| --- | --- |
| `cmd/dropwatch` | Process entry point and signal-aware root context |
| `internal/cli` | Cobra commands, flags, output, and exit behavior |
| `internal/tui` | Bubble Tea and Lip Gloss dashboard |
| `internal/app` | Composition root and domain-check orchestration |
| `internal/domain` | Domain types, normalization, and state mapping |
| `internal/rdap` | IANA bootstrap cache, HTTP client, and RDAP parsing |
| `internal/dnscheck` | Supplementary NS, SOA, A, AAAA, and MX checks |
| `internal/availability` | Provider extension contract and adapters |
| `internal/scheduler` | Adaptive intervals, backoff, watcher lease, and workers |
| `internal/notification` | Notifier contract, retry manager, Discord, and SMTP |
| `internal/storage` | Repository contracts and SQLite implementation |
| `internal/config` | Defaults, YAML, environment, and CLI override inputs |
| `internal/version` | Build metadata populated through linker flags |
| `migrations` | Embedded, ordered, forward-only SQL migrations |

## Check lifecycle

1. CLI input is normalized to Unicode and ASCII/Punycode.
2. The checker starts DNS and primary-provider work under the caller's context.
3. RDAP resolves the TLD endpoint through the cached IANA bootstrap, requires HTTPS, and validates redirects and dialed addresses against the public-network policy.
4. The application maps provider status to the internal state model.
5. Availability confirmations are persisted and evaluated for separation and count.
6. The interval calculator chooses the next check with jitter or error backoff.
7. SQLite records the check, updates the domain, and creates change events.
8. The notification manager creates one durable, deduplicated record per event and channel.
9. Each delivery attempt is persisted.

Caller cancellation during shutdown is returned without recording a false operational failure.

## Persistence

SQLite runs with foreign keys, WAL mode, a busy timeout, and a single long-lived connection so connection-local PRAGMAs remain effective. Network checks still run concurrently; only database operations are serialized.

Repository contracts live in `internal/storage/repository.go`. Business logic depends on these contracts rather than SQL. Schema changes must:

1. add a new numbered migration;
2. remain compatible with existing data;
3. include migration/repository tests;
4. update operational documentation when deployment behavior changes.

## Watcher and health

The watcher stores an owner token and heartbeat in `scheduler_state`. Acquisition is atomic and permits takeover only after the configured stale period. An in-memory set prevents the same process from checking one domain twice concurrently. Cancellation during a heartbeat follows the normal bounded worker shutdown before releasing the lease. Lease release uses a fresh, bounded context created at cleanup time, independent of watcher uptime.

The health command verifies:

- database connectivity;
- current migrations;
- write access;
- watcher presence and heartbeat freshness;
- absence of a stale scheduler lease.

The Docker healthcheck invokes this command directly.

## Extension points

- Add availability providers through `AvailabilityProvider`.
- Add notification channels through `Notifier`.
- Keep provider-specific configuration in `internal/config`.
- Construct new adapters in `internal/app`.
- Use local test servers or mocks; never make the unit suite depend on external services.
