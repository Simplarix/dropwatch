# Private production deployment with Dokploy

DropWatch is an outbound-only worker. It needs DNS, HTTPS, and optional SMTP egress, but it must never receive a public domain, proxy route, or published container port.

## 1. Secure GitHub and GHCR

1. Create `nbauditz/dropwatch` as a private repository.
2. Keep the GHCR package private and linked to the repository.
3. Enable the dependency graph, Dependabot alerts, Dependabot security updates, secret scanning, and push protection.
4. Protect `main` with required `CI` and `Security` checks, no force pushes, no deletion, and review before merge.
5. Protect `v*` tags with a repository ruleset that restricts tag creation, update, and deletion.
6. Create a `production` GitHub environment. Restrict it to protected tags and require manual approval.
7. Keep the default `GITHUB_TOKEN` permission read-only. The release workflow grants only the additional `contents` and `packages` permissions it needs.
8. If GitHub Dependency Review is available for the repository, require it on pull requests.

GitHub Actions are pinned to full commit SHAs. Dependabot monitors Go modules, Actions, and the pinned Docker base image. The security workflow runs source, dependency, configuration, and all-severity image vulnerability scans.

## 2. Publish an immutable release

Before tagging, ensure `main` is current and all checks pass:

```bash
make verify
make smoke
make security
git status
```

Create a signed or annotated stable tag:

```bash
git switch main
git pull --ff-only
git tag -s v0.1.0 -m "DropWatch v0.1.0"
git push origin v0.1.0
```

Only an exact `vMAJOR.MINOR.PATCH` tag whose commit is reachable from `origin/main` can release. The workflow:

1. runs the complete verification suite;
2. builds `amd64` and `arm64` binaries and SHA-256 checksums;
3. builds the multi-architecture image from digest-pinned build inputs;
4. publishes only the exact release tag;
5. publishes OCI provenance and an SBOM;
6. adds `image-digest.txt` to the GitHub Release.

Never delete, move, or rebuild a published version. Fixes use a new patch release.

## 3. Keep Dokploy private

Dokploy administration and its API should be reachable only through a private network such as WireGuard or Tailscale. Do not add Dokploy API credentials to GitHub.

The release workflow intentionally does not call Dokploy: a GitHub-hosted runner cannot reach a truly private control plane without exposing it. Perform the deployment through the private administration path, or use a dedicated, isolated self-hosted runner that is restricted to this private repository and production environment.

Protect the Dokploy account with a unique password and MFA when available. Restrict access to the Docker socket and Dokploy host to trusted administrators; Docker administrators can read container environment variables and control every container.

## 4. Configure the Compose application

Use [compose.production.yml](../compose.production.yml) as the Compose definition. It has:

- no `ports`, `expose`, domain, or reverse-proxy labels;
- a non-root user and read-only root filesystem;
- all Linux capabilities dropped and `no-new-privileges`;
- bounded processes, memory, CPU, temporary storage, and logs;
- a persistent `/data` volume and an ephemeral `/tmp`;
- a dedicated egress-capable bridge network.

From the release, copy the complete line in `image-digest.txt`, for example:

```text
ghcr.io/nbauditz/dropwatch@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

Set that exact value as the Dokploy project variable `DROPWATCH_IMAGE`. Floating tags such as `latest`, `v1`, or `v1.2` are not permitted.

For the private GHCR image, give Dokploy a dedicated read-only package credential. Do not reuse a personal token with repository write permissions.

Configure application values in Dokploy:

```text
DROPWATCH_DATABASE_PATH=/data/dropwatch.db
DROPWATCH_LOG_LEVEL=info
DROPWATCH_LOG_FORMAT=json
DROPWATCH_TIMEZONE=UTC
DROPWATCH_MAX_CONCURRENCY=4
DROPWATCH_HTTP_TIMEOUT=15s
```

Configure Discord or SMTP credentials only in Dokploy's secret/environment store. Keep `DROPWATCH_SMTP_REQUIRE_TLS=true`. Do not configure a domain, proxy, public port, or additional network.

## 5. Firewall and host policy

Use both the VPS provider firewall and the host firewall with default-deny inbound rules. Prefer administration over VPN; if public SSH is unavoidable, restrict it to known source addresses, require keys, disable password authentication, and disable direct root login.

DropWatch needs only the following outbound traffic:

- DNS over UDP/TCP 53 to the configured resolver;
- HTTPS over TCP 443 for IANA, RDAP, Discord, and GHCR image pulls;
- the configured SMTP port, normally TCP 587, only when SMTP is enabled.

No inbound rule is required for DropWatch. Remember that Docker manages firewall rules of its own; validate the effective `DOCKER-USER`/nftables policy after every Docker or host-firewall change.

Keep the VPS kernel, Docker Engine, Dokploy, and base operating system on supported versions with automatic security updates. Do not run unrelated public workloads on the same Docker network.

## 6. Deploy and verify

After changing `DROPWATCH_IMAGE` to the new digest, start the deployment manually in Dokploy through the private connection.

Verify on the VPS:

```bash
docker compose -f compose.production.yml config --quiet
docker ps --filter name=dropwatch
docker inspect --format '{{.Config.User}} {{.HostConfig.ReadonlyRootfs}} {{json .HostConfig.CapDrop}} {{json .NetworkSettings.Ports}}' CONTAINER
ss -lntup
```

Expected results:

- container user `10001:10001`;
- read-only root filesystem is `true`;
- every capability is dropped;
- the container has no published port;
- no DropWatch process appears in the host's listening-socket list;
- the image reference resolves to the approved digest;
- the healthcheck becomes healthy.

Then run `dropwatch notify test` inside the container and confirm that no credential or complete webhook URL appears in logs or notification-attempt records.

## 7. Data, backup, and rollback

The named `/data` volume contains SQLite data and the RDAP bootstrap cache. Back it up with a SQLite-aware snapshot or while the container is stopped. Encrypt backups, restrict their permissions, store them outside the VPS, and test restoration.

Rollback by setting `DROPWATCH_IMAGE` to a previously approved digest and redeploying. Never retag or overwrite an existing release.
