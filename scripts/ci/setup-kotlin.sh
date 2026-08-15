#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Install a pinned Kotlin compiler for CI. Usage:
#   sh scripts/ci/setup-kotlin.sh [version] [install_dir]
# Example version: 2.1.10

set -eu

VERSION="${1:-${CI_KOTLIN_VERSION:-2.1.10}}"
INSTALL_DIR="${2:-${KOTLIN_INSTALL_DIR:-$HOME/.local/kotlin}}"

OS="$(uname -s)"
case "$OS" in
	Linux|Darwin)
		;;
	*)
		echo "unsupported OS for CI Kotlin setup: $OS" >&2
		exit 1
		;;
esac

URL="https://github.com/JetBrains/kotlin/releases/download/v${VERSION}/kotlin-compiler-${VERSION}.zip"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Downloading $URL"
curl -fsSL "$URL" -o "$TMP/kotlin.zip"
mkdir -p "$INSTALL_DIR"
unzip -q "$TMP/kotlin.zip" -d "$TMP"

FOUND="$(find "$TMP" -type f -name kotlinc | head -n 1)"
if [ -z "$FOUND" ]; then
	echo "kotlinc not found in archive" >&2
	exit 1
fi

SRC_DIR="$(CDPATH='' cd -- "$(dirname "$FOUND")/.." && pwd)"
rm -rf "$INSTALL_DIR"
mkdir -p "$(dirname "$INSTALL_DIR")"
cp -a "$SRC_DIR" "$INSTALL_DIR"

BIN_DIR="${GITHUB_PATH_DIR:-$HOME/.local/bin}"
mkdir -p "$BIN_DIR"
ln -sfn "$INSTALL_DIR/bin/kotlinc" "$BIN_DIR/kotlinc"
ln -sfn "$INSTALL_DIR/bin/kotlin" "$BIN_DIR/kotlin"

if [ -n "${GITHUB_PATH:-}" ]; then
	echo "$BIN_DIR" >>"$GITHUB_PATH"
	echo "$INSTALL_DIR/bin" >>"$GITHUB_PATH"
fi

export PATH="$INSTALL_DIR/bin:$BIN_DIR:$PATH"
if [ -n "${GITHUB_ENV:-}" ]; then
	echo "KOTLIN_HOME=$INSTALL_DIR" >>"$GITHUB_ENV"
fi
echo "Kotlin installed: $(command -v kotlinc)"
kotlinc -version
echo "KOTLIN_HOME=$INSTALL_DIR"
