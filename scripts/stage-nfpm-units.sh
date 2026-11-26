#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum
# Copyright (c) 2024-2026 Quad4.io
#
# Substitute @BINDIR@ in init unit templates for nfpm packages.
# Packages always install the binary to /usr/bin.

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/dist/nfpm-units}"
BINDIR="${NFPM_BINDIR:-/usr/bin}"
PKG="$ROOT/packaging"

mkdir -p "$OUT/systemd" "$OUT/openrc" "$OUT/runit/log" "$OUT/dinit" "$OUT/completions"

subst() {
	sed "s|@BINDIR@|${BINDIR}|g" "$1" >"$2"
}

subst "$PKG/systemd/reticulum-go.service" "$OUT/systemd/reticulum-go.service"
subst "$PKG/openrc/reticulum-go" "$OUT/openrc/reticulum-go"
subst "$PKG/runit/reticulum-go/run" "$OUT/runit/run"
subst "$PKG/runit/reticulum-go/log/run" "$OUT/runit/log-run"
subst "$PKG/dinit/reticulum-go" "$OUT/dinit/reticulum-go"

chmod 644 "$OUT/systemd/reticulum-go.service" "$OUT/dinit/reticulum-go" "$OUT/runit/log-run"
chmod 755 "$OUT/openrc/reticulum-go" "$OUT/runit/run"

BIN="${BINARY:-$ROOT/bin/reticulum-go}"
if [ -x "$BIN" ]; then
	"$BIN" completion bash >"$OUT/completions/reticulum-go"
	"$BIN" completion zsh >"$OUT/completions/_reticulum-go"
	"$BIN" completion fish >"$OUT/completions/reticulum-go.fish"
	chmod 644 "$OUT/completions/reticulum-go" "$OUT/completions/_reticulum-go" "$OUT/completions/reticulum-go.fish"
else
	echo "missing built binary for shell completions: $BIN" >&2
	exit 1
fi
