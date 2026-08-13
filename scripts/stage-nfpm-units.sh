#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Substitute @BINDIR@ in init unit templates for nfpm packages.
# Packages always install the binary to /usr/bin.

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/dist/nfpm-units}"
BINDIR="${NFPM_BINDIR:-/usr/bin}"
PKG="$ROOT/packaging"

mkdir -p "$OUT/systemd" "$OUT/openrc" "$OUT/runit/log" "$OUT/dinit"

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
