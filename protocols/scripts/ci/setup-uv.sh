#!/bin/sh
# Install uv from Astral GitHub releases with SHA256 verification.
# Usage: setup-uv.sh <version>
set -eu

UV_VERSION="${1:?}"

ARCH="$(uname -m)"
case "$ARCH" in
x86_64) ARCH="x86_64" ;;
aarch64) ARCH="aarch64" ;;
*)
	echo "Unsupported architecture: $ARCH" >&2
	exit 1
	;;
esac

ASSET="uv-${ARCH}-unknown-linux-gnu.tar.gz"
BASE="https://github.com/astral-sh/uv/releases/download/${UV_VERSION}"

curl -fsSL "${BASE}/${ASSET}.sha256" -o /tmp/uv.sha256
curl -fsSL "${BASE}/${ASSET}" -o "/tmp/${ASSET}"
EXPECTED="$(awk '{print $1}' /tmp/uv.sha256)"
ACTUAL="$(sha256sum "/tmp/${ASSET}" | awk '{print $1}')"
if [ -z "$EXPECTED" ] || [ "$EXPECTED" != "$ACTUAL" ]; then
	echo "SHA256 verification failed for ${ASSET}" >&2
	exit 1
fi

INSTALL_DIR="${HOME:-/tmp}/.local/bin"
mkdir -p "$INSTALL_DIR"
tar -xzf "/tmp/${ASSET}" -C /tmp
install -m 0755 "/tmp/uv-${ARCH}-unknown-linux-gnu/uv" "$INSTALL_DIR/uv"
rm -rf "/tmp/${ASSET}" /tmp/uv.sha256 "/tmp/uv-${ARCH}-unknown-linux-gnu"

export PATH="${INSTALL_DIR}:$PATH"
if [ -n "${GITHUB_PATH:-}" ]; then
	echo "$INSTALL_DIR" >>"$GITHUB_PATH"
fi

uv --version
