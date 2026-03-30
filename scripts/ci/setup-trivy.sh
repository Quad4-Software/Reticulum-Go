#!/bin/sh
# Install Trivy from a .deb URL you pin (mirror or internal artifact store).
#
# Optional: TRIVY_DEB_SHA256 — if set, must match the file after download.
# Required: TRIVY_DEB_URL — direct URL to the .deb package.
set -eu

URL="${TRIVY_DEB_URL:-}"
EXPECTED="${TRIVY_DEB_SHA256:-}"

if [ -z "$URL" ]; then
    echo "setup-trivy.sh: set TRIVY_DEB_URL to a pinned .deb URL" >&2
    exit 1
fi

curl -fsSL -o /tmp/trivy.deb "$URL"

if [ -n "$EXPECTED" ]; then
    ACTUAL="$(sha256sum /tmp/trivy.deb | awk '{print $1}')"
    if [ "$ACTUAL" != "$EXPECTED" ]; then
        echo "SHA256 mismatch for Trivy .deb" >&2
        exit 1
    fi
fi

sudo dpkg -i /tmp/trivy.deb 2>/dev/null || sudo apt-get install -f -y
rm -f /tmp/trivy.deb
trivy version
