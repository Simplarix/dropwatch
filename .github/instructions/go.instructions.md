---
applyTo: "**/*.go"
---

# Go implementation rules

- Follow existing package boundaries and depend on repository/provider/notifier interfaces at business-logic boundaries.
- Prefer standard-library types and small focused functions; avoid global mutable state and unnecessary abstractions.
- Propagate `context.Context` through network and long-running operations.
- Wrap errors with operation context without including secrets or unfiltered external payloads.
- Store times in UTC and make clocks or randomness controllable in time-sensitive tests.
- Keep public JSON tags stable.
- Add table-driven tests for parsing and mapping, and temporary-database or local-server tests for infrastructure.
- Format with `gofmt` and run the affected package tests before the full `make verify`.

