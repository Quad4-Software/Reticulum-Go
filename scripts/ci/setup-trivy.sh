#!/bin/sh
# Install Trivy from the official Aqua Security GitHub release with a pinned SHA256.
#
# Usage: setup-trivy.sh <version>
#   version: e.g. 0.69.3
#
# Optional overrides:
#   TRIVY_DEB_URL     direct .deb URL (default: official GitHub release asset)
#   TRIVY_DEB_SHA256  expected SHA256 of the .deb (default: pinned per version)
set -eu

. "$(dirname "$0")/priv.sh"

VER="${1:-}"
if [ -z "$VER" ] && [ -n "${TRIVY_DEB_URL:-}" ]; then
    VER="custom"
fi
if [ -z "$VER" ]; then
    echo "setup-trivy.sh: usage: setup-trivy.sh <version> (e.g. 0.69.3)" >&2
    exit 1
fi

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)       echo "Unsupported architecture for Trivy .deb install: $ARCH" >&2; exit 1 ;;
esac

if [ "$ARCH" != "amd64" ]; then
    echo "setup-trivy.sh: official Trivy .deb packages target amd64; got $ARCH" >&2
    exit 1
fi

DEB="trivy_${VER}_Linux-64bit.deb"
URL="${TRIVY_DEB_URL:-https://github.com/aquasecurity/trivy/releases/download/v${VER}/${DEB}}"

case "$VER" in
    0.69.3) EXPECTED="${TRIVY_DEB_SHA256:-a484057aafde31089cf2558ca0f79a4bc835125a5ee6834183a5bcf0735af358}" ;;
    custom) EXPECTED="${TRIVY_DEB_SHA256:-}" ;;
    *)
        EXPECTED="${TRIVY_DEB_SHA256:-}"
        if [ -z "$EXPECTED" ]; then
            echo "setup-trivy.sh: no pinned SHA256 for Trivy ${VER}; set TRIVY_DEB_SHA256" >&2
            exit 1
        fi
        ;;
esac

curl -fsSL -o /tmp/trivy.deb "$URL"

if [ -n "$EXPECTED" ]; then
    ACTUAL="$(sha256sum /tmp/trivy.deb | awk '{print $1}')"
    if [ "$ACTUAL" != "$EXPECTED" ]; then
        echo "SHA256 mismatch for Trivy ${DEB}" >&2
        echo "expected: $EXPECTED" >&2
        echo "actual:   $ACTUAL" >&2
        rm -f /tmp/trivy.deb
        exit 1
    fi
fi

run_priv dpkg -i /tmp/trivy.deb 2>/dev/null || run_priv apt-get install -f -y
rm -f /tmp/trivy.deb
trivy version
