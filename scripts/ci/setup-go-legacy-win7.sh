#!/bin/sh
# Install go-legacy-win7 from GitHub releases with SHA256 pin.
# Usage: setup-go-legacy-win7.sh <version>
#   version: e.g. 1.26.4-1 (no "v" or "go-legacy-win7-" prefix)
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
    amd64) EXPECTED_SHA256="7884551101b9fe41db58c837174eef4fa95fa10edd41a5b8b3b4b043311d8a42" ;;
    arm64) EXPECTED_SHA256="37bac3a971b46a178e236fd7fc54c4ad12ac9245b0b0a73f8c7302ac513c8552" ;;
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
