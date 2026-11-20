#!/bin/sh
# Install goreleaser from GitHub releases with SHA256 verification.
# Usage: setup-goreleaser.sh [version]
set -eu

. "$(dirname "$0")/priv.sh"

VERSION="${1:-v2.19.0}"
VER="${VERSION#v}"

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="x86_64" ;;
    aarch64) ARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

BASE_URL="https://github.com/goreleaser/goreleaser/releases/download/${VERSION}"
ARCHIVE="goreleaser_Linux_${ARCH}.tar.gz"

curl -fsSL "${BASE_URL}/checksums.txt" -o /tmp/goreleaser-checksums.txt
curl -fsSL "${BASE_URL}/${ARCHIVE}" -o /tmp/goreleaser.tar.gz

EXPECTED="$(grep "  ${ARCHIVE}\$" /tmp/goreleaser-checksums.txt | awk '{print $1}')"
ACTUAL="$(sha256sum /tmp/goreleaser.tar.gz | awk '{print $1}')"
if [ "$EXPECTED" != "$ACTUAL" ]; then
	echo "goreleaser checksum mismatch: want $EXPECTED got $ACTUAL" >&2
	exit 1
fi

run_priv tar -xzf /tmp/goreleaser.tar.gz -C /usr/local/bin goreleaser
command -v goreleaser
goreleaser --version | head -3
