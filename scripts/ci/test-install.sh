#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Check install.sh with shellcheck and offline dry-run.
# Usage: sh scripts/ci/test-install.sh
# shellcheck shell=sh

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
INSTALL="$ROOT/install.sh"

fail() {
	printf 'test-install.sh: FAIL: %s\n' "$*" >&2
	exit 1
}

pass() {
	printf 'test-install.sh: %s\n' "$*"
}

if [ ! -f "$INSTALL" ]; then
	fail "missing $INSTALL"
fi

if ! command -v shellcheck >/dev/null 2>&1; then
	fail "shellcheck not found"
fi

pass "shellcheck install.sh"
shellcheck -s sh -x -S warning "$INSTALL"

pass "shellcheck scripts/install-service.sh"
shellcheck -s sh -x -S warning "$ROOT/scripts/install-service.sh"

help_out=$(sh "$INSTALL" --help)
case "$help_out" in
*"--dry-run"*) ;;
*) fail "help missing --dry-run" ;;
esac
case "$help_out" in
*"--binary"*) ;;
*) fail "help missing --binary" ;;
esac
case "$help_out" in
*"--source"*) ;;
*) fail "help missing --source" ;;
esac
case "$help_out" in
*"systemd"*) ;;
*) fail "help missing systemd" ;;
esac
case "$help_out" in
*"runit"*) ;;
*) fail "help missing runit" ;;
esac
pass "help text"

if sh "$INSTALL" --no-such-flag 2>/dev/null; then
	fail "unknown option should fail"
fi
pass "unknown option fails"

plan=$(sh "$INSTALL" --dry-run --binary --no-service --prefix /usr/local)
case "$plan" in
*"dry-run: no files will be written"*) ;;
*) fail "dry-run did not print no-write banner: $plan" ;;
esac
case "$plan" in
*"method=binary"*) ;;
*) fail "dry-run missing method=binary" ;;
esac
case "$plan" in
*"os="*" arch="*) ;;
*) fail "dry-run missing os/arch" ;;
esac
case "$plan" in
*"would skip service files"*) ;;
*) fail "dry-run --no-service still plans a service" ;;
esac
case "$plan" in
*"asset=reticulum-go-"*) ;;
*) fail "dry-run missing asset name" ;;
esac
pass "dry-run --binary --no-service"

plan=$(sh "$INSTALL" --dry-run --source --no-service --prefix /usr/local)
case "$plan" in
*"method=source"*) ;;
*) fail "dry-run missing method=source" ;;
esac
case "$plan" in
*"would install man pages"*) ;;
*) fail "source dry-run should mention man pages" ;;
esac
pass "dry-run --source --no-service"

plan=$(sh "$INSTALL" --dry-run --binary --init systemd --prefix /usr/local)
case "$plan" in
*"would install systemd service files"*) ;;
*) fail "dry-run --init systemd did not plan systemd: $plan" ;;
esac
pass "dry-run --init systemd"

plan=$(sh "$INSTALL" --dry-run --binary --no-service --goamd64 v1 --prefix /usr/local)
case "$plan" in
*"goamd64=v1"*) ;;
*) fail "forced v1 missing from plan: $plan" ;;
esac
case "$plan" in
*"asset=reticulum-go-linux-amd64-v3"*)
	fail "forced v1 still selected v3 asset"
	;;
esac
pass "dry-run --goamd64 v1"

stage="${TMPDIR:-/tmp}/rgo-install-dryrun-$$"
mkdir -p "$stage"
# Ensure dry-run never creates DESTDIR contents.
sh "$INSTALL" --dry-run --binary --service --destdir "$stage" --prefix /usr/local >/dev/null
if [ -e "$stage/usr" ] || [ -e "$stage/usr/local" ]; then
	rm -rf "$stage"
	fail "dry-run wrote files under DESTDIR"
fi
rm -rf "$stage"
pass "dry-run writes no DESTDIR files"

pass "ok"
