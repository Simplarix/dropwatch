# Security Policy

## Supported versions

Security fixes are provided for the latest stable release.

## Reporting a vulnerability

Do not publish suspected vulnerabilities, credentials, or exploit details in a public issue. Use GitHub Private Vulnerability Reporting in the repository's Security section. If that is unavailable, contact the repository maintainers through a private, verified channel.

A useful report includes the affected version, reproducible steps, impact, and a minimal fix proposal when available. Remove domain data, tokens, webhook URLs, SMTP credentials, and database contents from attachments.

## Secret handling

- `.env`, production overrides, private-domain data, and SQLite databases must not be committed.
- GitHub Actions has no Dokploy credentials and never contacts the private deployment control plane.
- Dokploy and GHCR tokens must have minimal permissions and be rotated regularly.
- Logs must not contain webhook URLs, passwords, API keys, or unfiltered external response bodies.
- Tests must use synthetic credentials and local servers.

## External input and network boundaries

- Every outbound request must use a context, timeout, redirect limit, and bounded response body.
- RDAP permits only public HTTPS targets. Redirects, DNS results, and literal addresses must not reach loopback, private, link-local, documentation, benchmarking, multicast, or metadata ranges.
- Domain input must be normalized and validated before it is placed in a URL or DNS query.
- External error bodies must not be copied into logs or user-facing errors.
- SMTP authentication must never silently downgrade to an insecure transport.

## Dependency and release security

- `make security` must pass before a production release.
- Dependabot monitors Go modules, GitHub Actions, and Docker references. Security updates must be reviewed promptly.
- GitHub Actions and Docker build inputs remain pinned to immutable identifiers.
- Releases originate only from protected stable tags whose commits belong to `main`.
- Production runs the exact digest recorded in the GitHub Release, never a floating tag.
- Dokploy, Docker, and the VPS remain private administration surfaces with default-deny inbound firewall policy.

## Operational limits

DropWatch is an observation tool. Its availability state does not guarantee that a domain can be registered or will remain available. Any critical action must be confirmed through an authoritative registrar interface.
