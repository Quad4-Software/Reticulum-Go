#!/bin/sh
# Install go-legacy-winxp from Quad4-Software GitHub releases.
# Usage: setup-go-legacy-winxp.sh <version>
#   version: e.g. 1.26.6 (no "v" or "go-legacy-winxp-" prefix)
set -eu

. "$(dirname "$0")/priv.sh"

VER="${1:?}"
INSTALL_ROOT="/usr/local/go-legacy-winxp"
BASE="https://github.com/Quad4-Software/go-legacy-winxp/releases/download/v${VER}"

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

TARBALL="go-legacy-winxp-${VER}.linux_${ARCH}.tar.gz"
URL="${BASE}/${TARBALL}"

case "$ARCH" in
    amd64) EXPECTED_SHA256="19787633c02d7c6c927fd500a548d6593447d8966133995a0055e0df00a75098" ;;
    arm64) EXPECTED_SHA256="196c28270281acb4d30e3e39f3c521a31cec2e644d1be42e5578ff3b25a81a01" ;;
esac

curl -fsSL "$URL" -o /tmp/go-legacy-winxp.tar.gz
ACTUAL_SHA256="$(sha256sum /tmp/go-legacy-winxp.tar.gz | awk '{print $1}')"
if [ "$ACTUAL_SHA256" != "$EXPECTED_SHA256" ]; then
	echo "SHA256 mismatch for ${TARBALL}" >&2
	rm -f /tmp/go-legacy-winxp.tar.gz
	exit 1
fi

run_priv rm -rf "$INSTALL_ROOT"
run_priv tar -C /usr/local -xzf /tmp/go-legacy-winxp.tar.gz
rm -f /tmp/go-legacy-winxp.tar.gz

export GOROOT="$INSTALL_ROOT"
export PATH="${INSTALL_ROOT}/bin:$PATH"
if [ -n "${GITHUB_PATH:-}" ]; then
	echo "${INSTALL_ROOT}/bin" >> "$GITHUB_PATH"
fi
if [ -n "${GITEA_PATH:-}" ]; then
	echo "${INSTALL_ROOT}/bin" >> "$GITEA_PATH"
fi

go version
