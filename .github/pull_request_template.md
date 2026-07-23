## Summary

Describe the user-visible or operational change.

## Safety and compatibility

- [ ] Availability decisions remain conservative.
- [ ] No secrets or unbounded external inputs were introduced.
- [ ] CLI JSON fields and exit behavior remain compatible, or the breaking change is documented.
- [ ] Persistence, migrations, concurrency, and restart behavior were considered.

## Validation

- [ ] Focused tests were added or updated.
- [ ] `make verify` passes.
- [ ] `make smoke` passes when relevant.
- [ ] `make compose-check` passes for Compose, environment, or deployment changes.
- [ ] `make security` passes for dependencies, networking, containers, workflows, or release-sensitive changes.
- [ ] Docker/workflow validation was performed when relevant.

## Deployment or user action

List migrations, configuration changes, secrets, rollout steps, or state that requires operator attention. Write “None” when not applicable.
