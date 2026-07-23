#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_dir=$(CDPATH= cd -- "${script_dir}/.." && pwd)
temporary_root=${TMPDIR:-/tmp}
smoke_dir=$(mktemp -d "${temporary_root%/}/dropwatch-smoke.XXXXXX")
watcher_pid=""

cleanup() {
	if [ -n "${watcher_pid}" ] && kill -0 "${watcher_pid}" 2>/dev/null; then
		kill -TERM "${watcher_pid}" 2>/dev/null || true
		wait "${watcher_pid}" 2>/dev/null || true
	fi
	case "${smoke_dir}" in
	"${temporary_root%/}"/dropwatch-smoke.*)
		rm -rf -- "${smoke_dir}"
		;;
	*)
		printf 'Refusing to remove unexpected smoke directory: %s\n' "${smoke_dir}" >&2
		;;
	esac
}

trap cleanup EXIT HUP INT TERM

go_command=${GO:-go}
binary="${smoke_dir}/dropwatch"
database="${smoke_dir}/dropwatch.db"
config="${smoke_dir}/missing-config.yaml"

cd "${repository_dir}"
"${go_command}" build -trimpath -o "${binary}" ./cmd/dropwatch

"${binary}" --database "${database}" --config "${config}" init --json >"${smoke_dir}/init.json"

"${binary}" --database "${database}" --config "${config}" watch >"${smoke_dir}/watch.log" 2>&1 &
watcher_pid=$!

health_ready=0
attempt=0
while [ "${attempt}" -lt 50 ]; do
	if "${binary}" --database "${database}" --config "${config}" health --quiet >/dev/null 2>&1; then
		health_ready=1
		break
	fi
	attempt=$((attempt + 1))
	sleep 0.1
done

if [ "${health_ready}" -ne 1 ]; then
	printf 'Watcher did not become healthy.\n' >&2
	sed -n '1,120p' "${smoke_dir}/watch.log" >&2
	exit 1
fi

kill -TERM "${watcher_pid}"
wait "${watcher_pid}"
watcher_pid=""

"${binary}" --database "${database}" --config "${config}" \
	domain add 'https://BÜCHER.de/a/path' --no-check --json >"${smoke_dir}/add.json"
"${binary}" --database "${database}" --config "${config}" \
	domain list --json >"${smoke_dir}/list.json"
"${binary}" --database "${database}" --config "${config}" \
	history 'bücher.de' --json >"${smoke_dir}/history.json"

grep -q '"ascii_domain": "xn--bcher-kva.de"' "${smoke_dir}/add.json"
grep -q '"state": "unknown"' "${smoke_dir}/list.json"
grep -q '"type": "domain_added"' "${smoke_dir}/history.json"

if [ "${LIVE_RDAP:-0}" = "1" ]; then
	printf 'Running live IANA/RDAP/DNS smoke check...\n'
	"${binary}" --database "${database}" --config "${config}" \
		domain add example.com --json >"${smoke_dir}/live.json"
	grep -q '"state": "registered"' "${smoke_dir}/live.json"
fi

printf 'DropWatch smoke test passed.\n'

