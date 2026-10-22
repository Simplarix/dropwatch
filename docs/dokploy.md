# Releases and Dokploy deployment

DropWatch is an outbound-only worker. It needs DNS, HTTPS, and optional SMTP
egress, but it must never receive a public domain, reverse-proxy route, or
published container port. GitHub publishes immutable release artifacts; Dokploy
pulls one approved image digest and deploys it manually.

## 1. Run the local release gate

Start from the repository root on a clean `main` branch:

```bash
git switch main
git pull --ff-only
make verify
make smoke
make smoke-live
make compose-check
make security
git status --short --branch
```

`make smoke-live` is the only optional command because it depends on public
IANA, RDAP, and DNS services. All other commands must pass. The final Git status
must show `main` aligned with `origin/main` and no uncommitted files.

Before each stable tag, confirm that:

- [CHANGELOG.md](../CHANGELOG.md) contains the intended version and date;
- all Dependabot pull requests have been reviewed, merged, or deliberately
  deferred;
- the commit intended for release is already on `main`;
- the `CI` and `Security` workflows are green for that exact commit.

## 2. Configure the GitHub repository

The canonical public repository is
[`Simplarix/dropwatch`](https://github.com/Simplarix/dropwatch).

### 2.1 General repository settings

Open **Repository → Settings → General** and configure:

1. Keep the default branch set to `main`.
2. Keep the repository public and retain the [MIT license](../LICENSE).
3. Under **Pull Requests**, enable squash merge and rebase merge, but disable
   merge commits if the `main` ruleset requires linear history. Use rebase merge
   when a pull request already contains deliberately separated commits; use
   squash merge for noisy work-in-progress histories.
4. Enable automatic deletion of head branches after a pull request is merged.
5. Keep Issues enabled if the included issue templates will be used.

Before publishing previously private material, review the complete Git history,
release assets, issues, and pull requests for credentials and private data. Keep
internal deployment notes and communication drafts outside the repository.

### 2.2 Actions permissions

Open **Repository → Settings → Actions → General**:

1. Allow GitHub-owned actions. The current workflows use only
   `actions/checkout` and `actions/setup-go`, both pinned to full commit SHAs.
2. Under **Workflow permissions**, select **Read repository contents and
   packages permissions**.
3. Leave **Allow GitHub Actions to create and approve pull requests** disabled.

The release workflow declares its own narrowly scoped `contents: write` and
`packages: write` permissions. CI and Security retain read-only permissions.

### 2.3 Protect `main` with a branch ruleset

Open **Repository → Settings → Rules → Rulesets → New ruleset → New branch
ruleset**:

1. Name it `main-protection`.
2. Set **Enforcement status** to **Active**.
3. Target the default branch, or include the exact pattern `main`.
4. Keep the bypass list empty unless an emergency administrator bypass is
   operationally required.
5. Enable:
   - restrict deletions;
   - block force pushes;
   - require linear history;
   - require a pull request before merging;
   - require conversation resolution before merging;
   - require status checks to pass;
   - require branches to be up to date before merging.
6. Add these three GitHub Actions checks from a recent successful run:
   - `verify`;
   - `Source and dependency scan`;
   - `Container and configuration scan`.

For a team repository, require at least one approval, dismiss stale approvals
after new commits, and require a CODEOWNER review. A sole maintainer cannot
approve their own pull request; in that case keep the pull-request and status
check requirements but do not create an approval rule that permanently blocks
the maintainer.

Do not enable **Restrict updates** for `main` unless the bypass list has been
designed carefully; the pull-request and required-check rules already prevent
unreviewed direct changes.

### 2.4 Protect stable release tags

Open **Repository → Settings → Rules → Rulesets → New ruleset → New tag
ruleset**:

1. Name it `stable-release-tags`.
2. Set **Enforcement status** to **Active**.
3. Include tags matching `v*`.
4. Add only the release maintainer or release team to the bypass list. Without
   this bypass, nobody can create the first stable tag.
5. Enable:
   - restrict creations;
   - restrict updates;
   - restrict deletions;
   - block force pushes.

The workflow independently rejects every tag that is not exactly
`vMAJOR.MINOR.PATCH` and every release commit that is not reachable from
`origin/main`.

### 2.5 Create the `production` environment

Open **Repository → Settings → Environments → New environment** and create an
environment named exactly `production`, matching the release workflow:

1. Under deployment branches and tags, select **Selected branches and tags**.
2. Add the tag pattern `v*`; do not add normal branches.
3. If required reviewers are available, add
   a release reviewer and prevent self-review when at least two maintainers are
   available.
4. Do not add Dokploy credentials or notification secrets. This environment
   gates publication only; the workflow never connects to Dokploy.

If the control is unavailable, the protected-tag bypass list and deliberate
creation of a signed stable tag are the manual release gate.

### 2.6 Enable repository security features

Open **Repository → Settings → Security & analysis** and enable every feature
available for the current plan:

- dependency graph;
- Dependabot alerts;
- Dependabot security updates;
- secret scanning;
- push protection;
- private vulnerability reporting.

If Dependency Review is available, require it for pull requests as an
additional `main` ruleset check. Keep the weekly Security workflow enabled.

## 3. Publish the immutable GitHub release

Choose the next unused stable version from [CHANGELOG.md](../CHANGELOG.md).
Replace `vX.Y.Z` below with that version; never reuse an existing tag. Create a
signed tag only after the protected `main` commit is green:

```bash
git switch main
git pull --ff-only
git status --short --branch
git tag -s vX.Y.Z -m "DropWatch vX.Y.Z"
git tag -v vX.Y.Z
git push origin vX.Y.Z
```

If signed tags are not yet configured, set up a verified GPG or SSH signing key
before releasing. The workflow also accepts an annotated tag, but a verifiable
signed release tag is preferred.

The `Stable release` workflow:

1. verifies the exact stable tag and its ancestry from `main`;
2. reruns verification, Compose validation, and the security gate;
3. builds Linux `amd64` and `arm64` binaries and checksums;
4. publishes `ghcr.io/simplarix/dropwatch:vX.Y.Z`;
5. attaches OCI provenance and an SBOM;
6. creates `image-digest.txt` with the immutable multi-architecture digest;
7. creates the GitHub Release and attaches all release files.

Approve the `production` job if the environment has a reviewer gate. After the
workflow finishes, verify that the GitHub Release contains:

- `dropwatch-linux-amd64`;
- `dropwatch-linux-arm64`;
- `checksums.txt`;
- `image-digest.txt`;
- `LICENSE` (from v0.1.10 onward).

The digest file must contain one complete line like:

```text
ghcr.io/simplarix/dropwatch@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

Never deploy the tag, `latest`, `v1`, or `v1.2`. Deploy only this digest line.
Never delete, move, or rebuild a published version; fixes use a new patch
release.

### 3.1 Verify the GHCR package

The first successful release creates the organization package. Open the
`dropwatch` package under the `Simplarix` organization and verify:

1. the package is linked to `Simplarix/dropwatch`;
2. the package visibility is public (separate from repository visibility);
3. repository Actions access permits `Simplarix/dropwatch` to publish;
4. the new version and its digest are visible;
5. the exact release digest can be pulled without registry credentials.

Set package visibility in **Organization → Packages → dropwatch → Package
settings → Change visibility → Public**. Verify an anonymous pull from a clean
Docker credential configuration before announcing container availability.

The workflow uses its short-lived `GITHUB_TOKEN`; no long-lived publishing token
belongs in GitHub secrets.

## 4. Prepare the Dokploy host

Keep the Dokploy UI, API, SSH, Docker socket, and host administration private.
Prefer WireGuard or Tailscale. If public SSH is unavoidable, restrict it by
source address, require keys, disable passwords, and disable direct root login.

Use both the VPS provider firewall and host firewall:

- no inbound rule is required for DropWatch;
- outbound UDP/TCP 53 is required for DNS;
- outbound TCP 443 is required for IANA, RDAP, Discord, GitHub, and GHCR;
- the configured SMTP port, normally TCP 587, is required only when SMTP is
  enabled.

Keep the host OS, kernel, Docker Engine, and Dokploy on supported versions with
security updates enabled. Remember that Docker manages firewall rules; validate
the effective `DOCKER-USER` or nftables policy after firewall changes.

## 5. Connect Dokploy to GitHub and GHCR

The repository and GHCR image are public. In Dokploy, use the GitHub provider or
public HTTPS Git URL `https://github.com/Simplarix/dropwatch.git` to read
`compose.production.yml`. The public image can be pulled without a registry token.

An existing read-only GitHub/registry connection may remain in use. Keep its
credentials private; no write access is needed to read a Compose definition or
pull an image. Public artifacts do not require public access to Dokploy, SSH,
Docker, the database, or notification credentials.

## 6. Create the Dokploy Compose service

In Dokploy:

1. Create project `dropwatch`.
2. Create environment `production`.
3. Add a **Compose** service named `dropwatch`.
4. Select **Docker Compose**, not Docker Stack.
5. Select the GitHub or Git provider configured above.
6. Select repository `Simplarix/dropwatch`, branch `main`, and Compose path
   `compose.production.yml`.
7. Disable automatic deployment from normal `main` pushes. Production rollout
   is intentionally manual after a successful stable release.
8. Do not configure a domain, Traefik route, proxy, published port, or additional
   public network.

The Compose definition already enforces:

- non-root user `10001:10001`;
- read-only root filesystem;
- all Linux capabilities dropped;
- `no-new-privileges`;
- bounded CPU, memory, processes, temporary storage, and logs;
- a named persistent `/data` volume;
- an ephemeral `/tmp`;
- no inbound port;
- a binary healthcheck.

## 7. Configure Dokploy environment variables

Open the Compose service **Environment** tab. Dokploy writes these values to its
deployment `.env`; `compose.production.yml` explicitly passes the supported
variables into the container.

Set:

```dotenv
DROPWATCH_IMAGE=ghcr.io/simplarix/dropwatch@sha256:REPLACE_WITH_RELEASE_DIGEST
DROPWATCH_DATABASE_PATH=/data/dropwatch.db
DROPWATCH_LOG_LEVEL=info
DROPWATCH_LOG_FORMAT=json
DROPWATCH_TIMEZONE=Europe/Berlin
DROPWATCH_MAX_CONCURRENCY=4
DROPWATCH_HTTP_TIMEOUT=15s
```

Replace `DROPWATCH_IMAGE` with the exact complete line from the GitHub Release.
`UTC` is also a valid timezone if local display is not required; storage always
remains UTC.

Configure at least one notification channel.

For Discord:

```dotenv
DROPWATCH_DISCORD_WEBHOOK_URL=
```

For SMTP:

```dotenv
DROPWATCH_SMTP_HOST=smtp.example.com
DROPWATCH_SMTP_PORT=587
DROPWATCH_SMTP_USERNAME=
DROPWATCH_SMTP_PASSWORD=
DROPWATCH_SMTP_FROM=dropwatch@example.com
DROPWATCH_SMTP_TO=operator@example.com
DROPWATCH_SMTP_REQUIRE_TLS=true
```

Fill the enabled channel’s empty credential values privately in Dokploy.
`DROPWATCH_SMTP_TO` accepts a comma-separated recipient list. Keep TLS required.
Omit the unused channel's variables instead of inventing placeholder
credentials. Treat Dokploy administrators as secret-bearing administrators
because Docker administrators can inspect container environments.

Before deployment, use Dokploy's rendered Compose preview or run
`docker compose config` and confirm that:

- the image is the approved `ghcr.io/simplarix/dropwatch@sha256:...` digest;
- the selected timezone and concurrency values are present;
- the enabled notification channel appears in the container environment;
- no ports, domains, or proxy labels appear.

## 8. Deploy and verify

Start the deployment manually. Wait for the container healthcheck to become
healthy. On the VPS, first identify the exact DropWatch container, then verify:

```bash
docker ps --filter name=dropwatch
docker inspect --format '{{.Config.Image}}' CONTAINER
docker inspect --format '{{.Config.User}} {{.HostConfig.ReadonlyRootfs}} {{json .HostConfig.CapDrop}} {{json .NetworkSettings.Ports}}' CONTAINER
docker exec CONTAINER dropwatch health --json
ss -lntup
```

Expected results:

- image is the approved immutable digest;
- user is `10001:10001`;
- read-only root filesystem is `true`;
- capability drop contains `ALL`;
- published ports are empty;
- health reports database, migrations, write access, watcher, and heartbeat as
  healthy;
- DropWatch does not appear as a listening host service.

Test notifications:

```bash
docker exec CONTAINER dropwatch notify test
docker logs CONTAINER
```

The command must report the configured channel as sent. Logs must not reveal a
password, complete webhook URL, or token.

Add the initial monitored domains only after health and notification checks:

```bash
docker exec CONTAINER dropwatch domain add example.com
docker exec CONTAINER dropwatch domain list
docker exec CONTAINER dropwatch history example.com --limit 20
```

Replace `example.com` with the intended domains. The default add command performs
an initial live check.

## 9. Backup, restore, and rollback

The Compose named volume contains:

- `/data/dropwatch.db`;
- SQLite WAL and shared-memory files while the process is running;
- the RDAP bootstrap cache.

Configure a Dokploy **Volume Backup** for the actual project-prefixed named
volume and an encrypted S3-compatible destination. A raw volume copy while
SQLite is writing is not guaranteed to be application-consistent. Use a
SQLite-aware backup or stop DropWatch cleanly before the volume snapshot. Never
copy only the database file while ignoring its WAL state.

Before calling the deployment production-ready:

1. create one backup;
2. restore it into an isolated test volume;
3. start the same image against the restored volume;
4. verify `dropwatch health`, the domain list, and history.

Rollback by setting `DROPWATCH_IMAGE` to a previously approved digest and
redeploying. Only roll back when that binary is compatible with all migrations
already applied to the persistent database. Never retag or overwrite an
existing release.

## 10. Production completion checklist

- GitHub `main` ruleset is active.
- Stable tag ruleset is active and has a usable release-maintainer bypass.
- `production` environment targets stable tags.
- Repository security features are enabled.
- Stable release workflow is green.
- GHCR package is public, linked, and anonymously pullable by release digest.
- Dokploy and host administration are private.
- Compose uses the release digest and has no inbound route.
- At least one notification channel passes its live test.
- Initial domains are present and have successful history.
- Healthcheck is healthy.
- Backup and isolated restore are proven.
