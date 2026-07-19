#!/bin/sh
# Install go-legacy-win7 from GitHub releases with SHA256 pin.
# Usage: setup-go-legacy-win7.sh <version>
#   version: e.g. 1.26.5-1 (no "v" or "go-legacy-win7-" prefix)
set -eu

. "$(dirname "$0")/priv.sh"

VER="${1:?}"
INSTALL_ROOT="/usr/local/go-legacy-win7"
BASE="https://github.com/thongtech/go-legacy-win7/releases/download/v${VER}"

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

TARBALL="go-legacy-win7-${VER}.linux_${ARCH}.tar.gz"
URL="${BASE}/${TARBALL}"

case "$ARCH" in
    amd64) EXPECTED_SHA256="4204dcd95218f61f4ffee51d0deef05a227228ae1f494755e855414271d7c4af" ;;
    arm64) EXPECTED_SHA256="126523010148d9c942c8bfba049d0a88c32a2a8047a3d56018549d5394a5eda2" ;;
esac

curl -fsSL "$URL" -o /tmp/go-legacy-win7.tar.gz
ACTUAL_SHA256="$(sha256sum /tmp/go-legacy-win7.tar.gz | awk '{print $1}')"
if [ "$ACTUAL_SHA256" != "$EXPECTED_SHA256" ]; then
    echo "SHA256 mismatch for ${TARBALL}" >&2
    rm -f /tmp/go-legacy-win7.tar.gz
    exit 1
fi

run_priv rm -rf "$INSTALL_ROOT"
run_priv tar -C /usr/local -xzf /tmp/go-legacy-win7.tar.gz
rm -f /tmp/go-legacy-win7.tar.gz

export GOROOT="$INSTALL_ROOT"
export PATH="${INSTALL_ROOT}/bin:$PATH"
if [ -n "${GITHUB_PATH:-}" ]; then
    echo "${INSTALL_ROOT}/bin" >> "$GITHUB_PATH"
fi
if [ -n "${GITEA_PATH:-}" ]; then
    echo "${INSTALL_ROOT}/bin" >> "$GITEA_PATH"
fi

go version
