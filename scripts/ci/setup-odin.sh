#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Install a pinned Odin release for CI. Usage:
#   sh scripts/ci/setup-odin.sh [tag] [install_dir]
# Example tag: dev-2026-06

set -eu

TAG="${1:-${CI_ODIN_VERSION:-dev-2026-06}}"
INSTALL_DIR="${2:-${ODIN_INSTALL_DIR:-$HOME/.local/odin}}"
ARCH="$(uname -m)"

case "$ARCH" in
	x86_64|amd64)
		ASSET_ARCH=amd64
		;;
	aarch64|arm64)
		ASSET_ARCH=arm64
		;;
	*)
		echo "unsupported arch: $ARCH" >&2
		exit 1
		;;
esac

OS="$(uname -s)"
case "$OS" in
	Linux)
		ASSET="odin-linux-${ASSET_ARCH}-${TAG}.tar.gz"
		;;
	*)
		echo "unsupported OS for CI Odin setup: $OS" >&2
		exit 1
		;;
esac

URL="https://github.com/odin-lang/Odin/releases/download/${TAG}/${ASSET}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Downloading $URL"
curl -fsSL "$URL" -o "$TMP/odin.tgz"
mkdir -p "$INSTALL_DIR"
tar -xzf "$TMP/odin.tgz" -C "$TMP"

# Release archives usually contain a top-level directory with the odin binary.
FOUND="$(find "$TMP" -type f -name odin | head -n 1)"
if [ -z "$FOUND" ]; then
	echo "odin binary not found in archive" >&2
	exit 1
fi

SRC_DIR="$(dirname "$FOUND")"
rm -rf "$INSTALL_DIR"
mkdir -p "$(dirname "$INSTALL_DIR")"
cp -a "$SRC_DIR" "$INSTALL_DIR"

BIN_DIR="${GITHUB_PATH_DIR:-$HOME/.local/bin}"
mkdir -p "$BIN_DIR"
ln -sfn "$INSTALL_DIR/odin" "$BIN_DIR/odin"

if [ -n "${GITHUB_PATH:-}" ]; then
	echo "$BIN_DIR" >> "$GITHUB_PATH"
	echo "$INSTALL_DIR" >> "$GITHUB_PATH"
fi

export PATH="$BIN_DIR:$INSTALL_DIR:$PATH"
odin version
