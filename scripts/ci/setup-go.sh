#!/bin/sh
# Install Go from dl.google.com with SHA256 from official checksum files.
# Usage: setup-go.sh <version>
#   version: e.g. 1.25.5 (no "go" prefix)
set -eu

GO_VER="${1:?}"
GO_VERSION="go${GO_VER#go}"

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

TARBALL="${GO_VERSION}.linux-${ARCH}.tar.gz"
BASE="https://dl.google.com/go"

curl -fsSL "${BASE}/${TARBALL}.sha256" | tr -d '\n\r ' > /tmp/go.sha256
EXPECTED="$(cat /tmp/go.sha256)"

curl -fsSL "${BASE}/${TARBALL}" -o /tmp/go.tar.gz
ACTUAL="$(sha256sum /tmp/go.tar.gz | awk '{print $1}')"
if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "SHA256 mismatch for ${TARBALL}" >&2
    rm -f /tmp/go.tar.gz /tmp/go.sha256
    exit 1
fi

sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf /tmp/go.tar.gz
rm -f /tmp/go.tar.gz /tmp/go.sha256

export PATH="/usr/local/go/bin:$PATH"
if [ -n "${GITHUB_PATH:-}" ]; then
    echo "/usr/local/go/bin" >> "$GITHUB_PATH"
fi
if [ -n "${GITEA_PATH:-}" ]; then
    echo "/usr/local/go/bin" >> "$GITEA_PATH"
fi

go version
