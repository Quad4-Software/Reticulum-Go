#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Install a pinned Swift toolchain for CI. Usage:
#   sh scripts/ci/setup-swift.sh [version] [install_dir]
# Example version: 6.0.3

set -eu

SWIFT_VERSION="${1:-${CI_SWIFT_VERSION:-6.0.3}}"
INSTALL_DIR="${2:-${SWIFT_INSTALL_DIR:-$HOME/.local/swift}}"
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
		;;
	*)
		echo "unsupported OS for CI Swift setup: $OS" >&2
		exit 1
		;;
esac

# os-release defines VERSION and would overwrite SWIFT_VERSION if named VERSION.
. /etc/os-release 2>/dev/null || true
case "${VERSION_ID:-}" in
	24.04*)
		PLATFORM=ubuntu2404
		ASSET_OS=ubuntu24.04
		;;
	22.04*)
		PLATFORM=ubuntu2204
		ASSET_OS=ubuntu22.04
		;;
	20.04*)
		PLATFORM=ubuntu2004
		ASSET_OS=ubuntu20.04
		;;
	*)
		# GitHub ubuntu-latest is currently 24.04-class. Fall back.
		PLATFORM=ubuntu2404
		ASSET_OS=ubuntu24.04
		echo "warning: unknown VERSION_ID=${VERSION_ID:-}, using ${PLATFORM}" >&2
		;;
esac

# Allow override for non-Ubuntu hosts testing against a glibc-compatible toolchain.
PLATFORM="${SWIFT_PLATFORM:-$PLATFORM}"
ASSET_OS="${SWIFT_ASSET_OS:-$ASSET_OS}"

if [ "$ASSET_ARCH" = aarch64 ]; then
	case "$PLATFORM" in
		*-aarch64) ;;
		*) PLATFORM="${PLATFORM}-aarch64" ;;
	esac
	case "$ASSET_OS" in
		*-aarch64) ;;
		*) ASSET_OS="${ASSET_OS}-aarch64" ;;
	esac
fi

ASSET="swift-${SWIFT_VERSION}-RELEASE-${ASSET_OS}.tar.gz"
URL="https://download.swift.org/swift-${SWIFT_VERSION}-release/${PLATFORM}/swift-${SWIFT_VERSION}-RELEASE/${ASSET}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Downloading $URL"
curl -fsSL "$URL" -o "$TMP/swift.tar.gz"
mkdir -p "$INSTALL_DIR"
tar -xzf "$TMP/swift.tar.gz" -C "$TMP"

FOUND="$(find "$TMP" \( -type f -o -type l \) -name swift | head -n 1)"
if [ -z "$FOUND" ]; then
	echo "swift binary not found in archive" >&2
	exit 1
fi

SRC_DIR="$(CDPATH='' cd -- "$(dirname "$FOUND")/.." && pwd)"
rm -rf "$INSTALL_DIR"
mkdir -p "$(dirname "$INSTALL_DIR")"
cp -a "$SRC_DIR" "$INSTALL_DIR"

BIN_DIR="${GITHUB_PATH_DIR:-$HOME/.local/bin}"
mkdir -p "$BIN_DIR"
ln -sfn "$INSTALL_DIR/usr/bin/swift" "$BIN_DIR/swift"
ln -sfn "$INSTALL_DIR/usr/bin/swiftc" "$BIN_DIR/swiftc"

if [ -n "${GITHUB_PATH:-}" ]; then
	echo "$BIN_DIR" >>"$GITHUB_PATH"
	echo "$INSTALL_DIR/usr/bin" >>"$GITHUB_PATH"
fi

export PATH="$INSTALL_DIR/usr/bin:$BIN_DIR:$PATH"
echo "Swift installed: $(command -v swift)"
swift --version | head -n 2
