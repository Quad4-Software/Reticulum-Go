#!/bin/sh
# Fail if gofmt would change tracked Go sources (vendor excluded).
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

. "$ROOT/scripts/ci/env.sh"
export PATH="/usr/local/go/bin:${PATH}"

DIRS="./pkg ./internal ./examples"
OUT="$(gofmt -l $DIRS 2>/dev/null || true)"
if [ -n "$OUT" ]; then
	echo "gofmt would reformat:" >&2
	echo "$OUT" >&2
	exit 1
fi
echo "fmt-check: OK"
