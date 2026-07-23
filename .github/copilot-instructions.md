# DropWatch Repository Instructions

Treat `AGENTS.md` as the authoritative repository guide and `REVIEW.md` as the review checklist.

- This is a Go 1.26.5 CLI, TUI, and worker using Cobra, Bubble Tea, Lip Gloss, and SQLite.
- Preserve the conservative availability invariant: DNS never decides availability; errors never become `available`; RDAP 404 needs separated confirmations; registration signals win.
- Keep business logic in `internal/app` and `internal/domain`, persistence behind `internal/storage` interfaces, and presentation in `internal/cli` or `internal/tui`.
- Use contexts, timeouts, redirect limits, bounded response bodies, UTC storage, and secret-safe errors for all external boundaries.
- Add forward-only migrations instead of editing applied SQL files.
- Add deterministic tests with fixtures, temporary databases, mocks, or local servers. Do not add live-network tests to CI.
- Preserve stable CLI JSON fields, exit codes, graceful shutdown, notification deduplication, watcher leasing, and non-root Docker behavior.
- Run focused tests while editing, then `make verify`; run `make smoke` for CLI, config, storage, scheduler, or health changes.
- Never push, tag, publish, deploy, or add real secrets unless explicitly instructed.

