# Contributing

Contributions from humans and coding agents are welcome.

## Before changing code

1. Read [AGENTS.md](AGENTS.md).
2. Review [docs/architecture.md](docs/architecture.md) for package ownership and invariants.
3. Keep the change focused and preserve unrelated user work.
4. Add or update deterministic tests for behavior changes.

## Required validation

Run:

```bash
make verify
make smoke
make security
```

Run `make smoke-live` only when the change affects the real bootstrap, RDAP, or DNS path and outbound network access is appropriate. Run `make docker-build` and `make compose-check` for Dockerfile or runtime-image changes. `make security` needs current vulnerability data and is required before a production release.

## Engineering expectations

- Keep business logic, storage, transport, CLI, and TUI concerns separated.
- Do not make tests depend on live RDAP or public DNS services.
- Keep availability decisions conservative: network failures, rate limits, DNS failures, and ambiguous responses must never produce `available`.
- Add new schema changes as forward-only embedded migrations; do not rewrite an applied migration.
- Persist and deduplicate every notification delivery attempt.
- Keep secrets out of examples, errors, fixtures, logs, and commits.
- Document new configuration in `config.example.yaml`, `.env.example`, and README.
- Preserve stable JSON fields and exit-code behavior unless a breaking change is intentional and documented.

## Pull requests

Use the pull-request template. Explain the behavior change, safety impact, verification performed, and any operational or migration consequences. Commits should be reviewable and must not include generated binaries, databases, caches, real credentials, or local environment files.
