#!/bin/sh
# Verify reticulum-go.rsm signature and byte-level file hashes.
#
# Env:
#   RNS_REQUIRED_SIGNER  identity hash (default: e46112d44649266d71fe2193e00a4710)
#   RNS_RSM_PATH         path to .rsm (default: reticulum-go.rsm)
#   RNS_ID_BIN           reticulum-go binary (default: bin/reticulum-go)
#   RNS_INVENTORY_OUT    if set, write extracted inventory here (for end-of-job recheck)
#
# Usage:
#   sh scripts/ci/verify-tree-rsm.sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

SIGNER="${RNS_REQUIRED_SIGNER:-e46112d44649266d71fe2193e00a4710}"
RSM_PATH="${RNS_RSM_PATH:-$ROOT/reticulum-go.rsm}"
BIN="${RNS_ID_BIN:-$ROOT/bin/reticulum-go}"

if [ ! -f "$RSM_PATH" ]; then
	echo "verify-tree-rsm.sh: missing $RSM_PATH" >&2
	exit 1
fi

if [ ! -x "$BIN" ]; then
	echo "verify-tree-rsm.sh: building reticulum-go..."
	(cd "$ROOT" && go build -mod=vendor -o "$BIN" ./cmd/reticulum-go)
fi

INV="$(mktemp "${TMPDIR:-/tmp}/tree-inv-verify.XXXXXX")"
trap 'rm -f "$INV"' EXIT INT

# Cryptographic verify + extract embedded inventory (public signer hash only).
if ! "$BIN" id -i "$SIGNER" -V "$RSM_PATH" -extract >"$INV"; then
	echo "verify-tree-rsm.sh: RSM signature verification failed" >&2
	exit 1
fi

if [ -n "${RNS_INVENTORY_OUT:-}" ]; then
	cp "$INV" "$RNS_INVENTORY_OUT"
fi

sh "$ROOT/scripts/ci/tree-manifest.sh" verify-tracked "$INV"
echo "verify-tree-rsm.sh: OK (signer $SIGNER)"
