#!/bin/sh
# Install go-legacy-winxp from Quad4-Software GitHub releases.
# Usage: setup-go-legacy-winxp.sh <version>
#   version: e.g. 1.27.1 (no "v" or "go-legacy-winxp-" prefix)
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
    amd64) EXPECTED_SHA256="c627806bccb8c5ec2fc43041a2f01a231d4b69eb200c0baffc10e709be4af030" ;;
    arm64) EXPECTED_SHA256="f38ba248edeca69ab9bff69f5c80d325a0d6e7068288210a956dfc36ae936afe" ;;
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
