#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Install a pinned Zig release for CI. Usage:
#   sh scripts/ci/setup-zig.sh [version] [install_dir]
# Example version: 0.16.0

set -eu

VERSION="${1:-${CI_ZIG_VERSION:-0.16.0}}"
INSTALL_DIR="${2:-${ZIG_INSTALL_DIR:-$HOME/.local/zig}}"
ARCH="$(uname -m)"

case "$ARCH" in
	x86_64|amd64)
		ASSET_ARCH=x86_64
		;;
	aarch64|arm64)
		ASSET_ARCH=aarch64
		;;
	*)
		echo "unsupported arch: $ARCH" >&2
		exit 1
		;;
esac

OS="$(uname -s)"
case "$OS" in
	Linux)
		ASSET="zig-${ASSET_ARCH}-linux-${VERSION}.tar.xz"
		;;
	*)
		echo "unsupported OS for CI Zig setup: $OS" >&2
		exit 1
		;;
esac

URL="https://ziglang.org/download/${VERSION}/${ASSET}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Downloading $URL"
curl -fsSL "$URL" -o "$TMP/zig.tar.xz"
mkdir -p "$INSTALL_DIR"
tar -xJf "$TMP/zig.tar.xz" -C "$TMP"

FOUND="$(find "$TMP" -type f -name zig | head -n 1)"
if [ -z "$FOUND" ]; then
	echo "zig binary not found in archive" >&2
	exit 1
fi

SRC_DIR="$(dirname "$FOUND")"
rm -rf "$INSTALL_DIR"
mkdir -p "$(dirname "$INSTALL_DIR")"
cp -a "$SRC_DIR" "$INSTALL_DIR"

BIN_DIR="${GITHUB_PATH_DIR:-$HOME/.local/bin}"
mkdir -p "$BIN_DIR"
ln -sfn "$INSTALL_DIR/zig" "$BIN_DIR/zig"

if [ -n "${GITHUB_PATH:-}" ]; then
	echo "$BIN_DIR" >> "$GITHUB_PATH"
	echo "$INSTALL_DIR" >> "$GITHUB_PATH"
fi

export PATH="$BIN_DIR:$INSTALL_DIR:$PATH"
zig version
