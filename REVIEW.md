# Review Checklist

Review the change for correctness, security, persistence, and operational compatibility. Report concrete findings with file and line references; do not report formatting issues already enforced by CI.

## Availability safety

- No error, timeout, cancellation, rate limit, 5xx response, malformed payload, or DNS failure can become `available`.
- RDAP 404 remains `possibly_available` until separated confirmation requirements are met.
- Registration signals override availability signals.
- Hold, redemption, and pending-delete mappings remain conservative.

## Persistence and concurrency

- State, checks, events, confirmations, delivery attempts, and heartbeat changes are transactional where required.
- Schema changes use a new forward-only migration and preserve existing data.
- Watcher leasing prevents concurrent owners; per-domain in-flight protection remains intact.
- Shutdown cancellation does not create misleading check failures or abandon the watcher lease.

## External boundaries and secrets

- HTTP, DNS, and SMTP operations use contexts, timeouts, and bounded inputs.
- Redirect and response-size limits remain enforced.
- RDAP permits only public HTTPS destinations and rejects private, loopback, link-local, metadata, downgrade, and unsafe redirect targets.
- URLs contain only normalized domains.
- Logs and errors cannot reveal passwords, API keys, webhook URLs, or raw external bodies.
- Notification delivery remains durable and deduplicated.

## Compatibility and operations

- CLI exit codes and stable JSON fields remain compatible.
- Internal timestamps remain UTC.
- Docker stays non-root and retains CA certificates, timezone data, `/data`, and the binary healthcheck.
- Production Compose keeps a read-only root filesystem, drops all capabilities, uses `no-new-privileges`, and references an immutable image digest.
- Production Compose explicitly passes every documented Dokploy runtime and notification variable into the container.
- CI never deploys.
- Releases remain limited to exact stable `vMAJOR.MINOR.PATCH` tags on commits reachable from `main`; private deployment is a separate operator action.

## Validation

- Tests cover the changed success and failure paths.
- No deterministic test depends on live RDAP or public DNS.
- `make verify` passes.
- `make smoke` and Docker validation run when relevant.
- `make compose-check` passes for deployment configuration changes.
- `make security` passes before release.
