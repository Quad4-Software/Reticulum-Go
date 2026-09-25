#!/bin/sh
# Install TinyGo .deb from GitHub releases with optional SHA256 pin (TINYGO_DEB_SHA256).
# Usage: setup-tinygo.sh <version> [arch]
set -eu

. "$(dirname "$0")/priv.sh"

VER="${1:?}"
ARCH="${2:-amd64}"
DEB="tinygo_${VER}_${ARCH}.deb"
URL="https://github.com/tinygo-org/tinygo/releases/download/v${VER}/${DEB}"

curl -fsSL -o /tmp/tinygo.deb "$URL"

if [ -n "${TINYGO_DEB_SHA256:-}" ]; then
    ACTUAL="$(sha256sum /tmp/tinygo.deb | awk '{print $1}')"
    if [ "$ACTUAL" != "$TINYGO_DEB_SHA256" ]; then
        echo "SHA256 mismatch for TinyGo ${DEB}" >&2
        rm -f /tmp/tinygo.deb
        exit 1
    fi
fi

run_priv dpkg -i /tmp/tinygo.deb 2>/dev/null || run_priv apt-get install -f -y
rm -f /tmp/tinygo.deb
GOT="$(tinygo version 2>&1 | head -n1)"
echo "$GOT"
case "$GOT" in
    *"$VER"*) ;;
    *) echo "error: resolved TinyGo does not match pinned ${VER} (got: $GOT)" >&2; exit 1 ;;
esac
