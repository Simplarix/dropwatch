#!/bin/sh
set -eu

# Keep release archives pinned independently of the download endpoint.
version=0.72.0
case "$(uname -s)-$(uname -m)" in
	Linux-x86_64)
		platform=Linux-64bit
		checksum=bbb64b9695866ce4a7a8f5c9592002c5961cab378577fa3f8a040df362b9b2ea
		;;
	Linux-aarch64|Linux-arm64)
		platform=Linux-ARM64
		checksum=2ca2c023109c2db6b2b77366b6717291452d4531167377d95c79547f0c8e3467
		;;
	Darwin-x86_64)
		platform=macOS-64bit
		checksum=ee5e60df8a98e5b89fd74a6d86f9e5c7e9a266a35002cb1e43291698b3bfee08
		;;
	Darwin-arm64)
		platform=macOS-ARM64
		checksum=88f208680dc05da2b459e19b4f5aa2b4dc7c2117892ba4aab2ae63baba330016
		;;
	*)
		printf '%s\n' 'Unsupported Trivy platform; use Linux or macOS on amd64 or arm64.' >&2
		exit 1
		;;
esac

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tools_dir=${DROPWATCH_TOOLS_DIR:-"${repo_dir}/.cache/tools"}
binary="${tools_dir}/trivy-${version}-${platform}"

if [ ! -x "$binary" ]; then
	mkdir -p "$tools_dir"
	tmp_dir=$(mktemp -d "${tools_dir}/.trivy-install.XXXXXX")
	trap 'rm -rf "$tmp_dir"' EXIT
	trap 'exit 1' HUP INT TERM
	archive="trivy_${version}_${platform}.tar.gz"
	printf 'Downloading Trivy %s (%s)\n' "$version" "$platform" >&2
	curl --fail --silent --show-error --location \
		--proto '=https' --proto-redir '=https' --max-redirs 5 \
		--connect-timeout 10 --max-time 120 --retry 2 --max-filesize 134217728 \
		--output "${tmp_dir}/${archive}" \
		"https://github.com/aquasecurity/trivy/releases/download/v${version}/${archive}"
	(
		cd "$tmp_dir"
		if command -v sha256sum >/dev/null 2>&1; then
			actual=$(sha256sum < "$archive")
		else
			actual=$(shasum -a 256 < "$archive")
		fi
		if [ "${actual%% *}" != "$checksum" ]; then
			printf '%s\n' 'Trivy archive checksum mismatch; refusing to install.' >&2
			exit 1
		fi
		tar -xzf "$archive" trivy
	)
	chmod 0755 "${tmp_dir}/trivy"
	mv "${tmp_dir}/trivy" "$binary"
	rm -rf "$tmp_dir"
	trap - EXIT HUP INT TERM
fi

exec "$binary" "$@"
