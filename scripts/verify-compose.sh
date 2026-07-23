#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
rendered=$(mktemp)
trap 'rm -f "$rendered"' EXIT HUP INT TERM

release_digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
release_image="ghcr.io/simplarix/dropwatch@${release_digest}"
synthetic_webhook=https://discord.invalid/api/webhooks/synthetic

DROPWATCH_IMAGE="$release_image" \
DROPWATCH_DATABASE_PATH=/data/custom.db \
DROPWATCH_LOG_LEVEL=debug \
DROPWATCH_LOG_FORMAT=text \
DROPWATCH_TIMEZONE=Europe/Berlin \
DROPWATCH_MAX_CONCURRENCY=7 \
DROPWATCH_HTTP_TIMEOUT=21s \
DROPWATCH_DISCORD_WEBHOOK_URL="$synthetic_webhook" \
DROPWATCH_SMTP_HOST=smtp.example.com \
DROPWATCH_SMTP_PORT=2525 \
DROPWATCH_SMTP_USERNAME=synthetic-user \
DROPWATCH_SMTP_PASSWORD=synthetic-password \
DROPWATCH_SMTP_FROM=from@example.com \
DROPWATCH_SMTP_TO=to@example.com \
DROPWATCH_SMTP_REQUIRE_TLS=true \
	docker compose -f "${repo_dir}/compose.production.yml" config >"$rendered"

require_line() {
	if ! grep -Fq "$1" "$rendered"; then
		printf 'rendered Compose configuration is missing: %s\n' "$1" >&2
		exit 1
	fi
}

require_line "image: ${release_image}"
require_line "DROPWATCH_DATABASE_PATH: /data/custom.db"
require_line "DROPWATCH_LOG_LEVEL: debug"
require_line "DROPWATCH_LOG_FORMAT: text"
require_line "DROPWATCH_TIMEZONE: Europe/Berlin"
require_line 'DROPWATCH_MAX_CONCURRENCY: "7"'
require_line "DROPWATCH_HTTP_TIMEOUT: 21s"
require_line "DROPWATCH_DISCORD_WEBHOOK_URL: ${synthetic_webhook}"
require_line "DROPWATCH_SMTP_HOST: smtp.example.com"
require_line 'DROPWATCH_SMTP_PORT: "2525"'
require_line "DROPWATCH_SMTP_USERNAME: synthetic-user"
require_line "DROPWATCH_SMTP_PASSWORD: synthetic-password"
require_line "DROPWATCH_SMTP_FROM: from@example.com"
require_line "DROPWATCH_SMTP_TO: to@example.com"
require_line 'DROPWATCH_SMTP_REQUIRE_TLS: "true"'

printf '%s\n' "DropWatch production Compose validation passed."
