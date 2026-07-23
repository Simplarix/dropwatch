---
applyTo: "Dockerfile,compose.production.yml,.github/workflows/*.yml,.github/workflows/*.yaml"
---

# Deployment rules

- Keep the runtime non-root, CGO-free, minimal, and equipped with CA certificates and timezone data.
- Preserve `/data` as the only persistent application directory and keep the binary healthcheck.
- Do not add a public port; DropWatch is a worker.
- Never place credentials in Docker layers, Compose, workflow output, or examples.
- Keep CI read-only and deployment-free.
- Release only from exact stable `vMAJOR.MINOR.PATCH` tag pushes.
- Require the release commit to be reachable from `main`; keep GitHub Actions pinned to full commit SHAs.
- Publish only the exact stable image tag, attach provenance and an SBOM, and deploy the recorded immutable digest.
- Keep Dokploy private. GitHub-hosted runners must never call its API; deployment is manual over the private administration path.
- Preserve `amd64` and `arm64` binaries, checksums, OCI labels, and the release image digest.
