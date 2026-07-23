# Agentic Development

DropWatch is configured for Codex, Claude Code, and GitHub Copilot without maintaining three conflicting copies of project policy.

## Instruction hierarchy

1. [AGENTS.md](../AGENTS.md) is the repository-wide source of truth.
2. [CLAUDE.md](../CLAUDE.md) imports `AGENTS.md` for Claude Code.
3. [.github/copilot-instructions.md](../.github/copilot-instructions.md) gives Copilot a concise entry point and points back to `AGENTS.md`.
4. Path-specific Copilot files under `.github/instructions/` add only file-type guidance.
5. [REVIEW.md](../REVIEW.md) contains the detailed review checklist.
6. Architecture and testing details live in `docs/` so always-loaded instruction files stay compact.

When guidance conflicts, follow the instruction closest to the edited file, then `AGENTS.md`. User instructions for the current task still take precedence.

## Recommended agent workflow

1. Read `AGENTS.md`.
2. Inspect only the architecture and testing sections relevant to the task.
3. Check the working tree before editing and preserve unrelated changes.
4. State assumptions that could affect behavior or deployment.
5. Make the smallest coherent change.
6. Add deterministic tests for changed behavior.
7. Run focused tests during iteration.
8. Run `make verify` before handoff.
9. Run `make smoke` when CLI, storage, scheduler, or configuration changes.
10. Run `make security` before a production release or after dependency, networking, container, or workflow changes.
11. Review the diff against `REVIEW.md`.
12. Report changed behavior, validation, and any remaining user action.

## Planning larger changes

For work spanning three or more packages, create a short execution plan in the task or a temporary local note. A useful plan includes:

```text
Goal:
In scope:
Out of scope:
Safety invariants:
Files/packages likely affected:
Migration or compatibility impact:
Test strategy:
Deployment impact:
```

Update the plan as facts change. Do not leave stale plan files in the repository unless they document an accepted design decision.

## Context discipline

- Prefer `rg` and focused file reads over loading the whole repository.
- Read interfaces and tests before implementation details.
- Do not repeat architecture prose inside tool-specific instruction files.
- Keep agent guidance concrete, verifiable, and under the relevant context limits.
- If an agent repeats a mistake, update `AGENTS.md` or the closest path-specific rule with a short preventive instruction.

## Safe autonomy

Agents may freely inspect, build, test, and edit files within the requested scope. They must not:

- push commits, publish releases, or trigger Dokploy unless explicitly requested;
- add secrets or use real notification endpoints for tests;
- rewrite release tags or existing migrations;
- treat live-network output as a deterministic test oracle;
- delete or overwrite unrelated user work;
- broaden a feature request into deployment or infrastructure changes without authorization.

## Tool-specific verification

- Codex: ask it to summarize active repository instructions and ensure it names `AGENTS.md`.
- Claude Code: run `/memory` and confirm that `CLAUDE.md` and its `AGENTS.md` import are loaded.
- GitHub Copilot: inspect response references for `.github/copilot-instructions.md`; path-specific files should appear when matching files are edited.
