#!/bin/sh
# Install go-legacy-win7 from GitHub releases with SHA256 pin.
# Usage: setup-go-legacy-win7.sh <version>
#   version: e.g. 1.27.2-1 (no "v" or "go-legacy-win7-" prefix)
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
    amd64) EXPECTED_SHA256="55e030fa608b782d6f19ad121c696f115fb3fb1833baab57d611c1f78ebf5a88" ;;
    arm64) EXPECTED_SHA256="b6da0d28b8b203d2c02c08b0f8fcf0c9f531473a982a9b80860b5f039779af80" ;;
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
