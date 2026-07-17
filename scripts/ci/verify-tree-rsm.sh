#!/bin/sh
# Verify reticulum-go.rsm signature and byte-level file hashes.
#
# Env:
#   RNS_REQUIRED_SIGNER  identity hash (default: e46112d44649266d71fe2193e00a4710)
#   RNS_RSM_PATH         path to .rsm (default: reticulum-go.rsm)
#   RNS_ID_BIN           reticulum-go binary (default: bin/reticulum-go)
#   RNS_INVENTORY_OUT    if set, write inventory here only after hash verify succeeds
#   RNS_TREE_VERIFY_OPTIONAL  if 1 or true, warn and exit 0 on failure (CI soft check)
#
# Usage:
#   sh scripts/ci/verify-tree-rsm.sh
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

SIGNER="${RNS_REQUIRED_SIGNER:-e46112d44649266d71fe2193e00a4710}"
RSM_PATH="${RNS_RSM_PATH:-$ROOT/reticulum-go.rsm}"
BIN="${RNS_ID_BIN:-$ROOT/bin/reticulum-go}"

warn_or_fail() {
	msg="$1"
	case "${RNS_TREE_VERIFY_OPTIONAL:-}" in
	1 | true | TRUE | yes | YES)
		echo "::warning::verify-tree-rsm.sh: $msg (optional, continuing)"
		exit 0
		;;
	*)
		echo "verify-tree-rsm.sh: $msg" >&2
		exit 1
		;;
	esac
}

if [ ! -f "$RSM_PATH" ]; then
	warn_or_fail "missing $RSM_PATH"
fi

if [ ! -x "$BIN" ]; then
	echo "verify-tree-rsm.sh: building reticulum-go..."
	(cd "$ROOT" && go build -mod=vendor -o "$BIN" ./cmd/reticulum-go)
fi

INV="$(mktemp "${TMPDIR:-/tmp}/tree-inv-verify.XXXXXX")"
trap 'rm -f "$INV"' EXIT INT

# Cryptographic verify + extract embedded inventory (public signer hash only).
if ! "$BIN" id -i "$SIGNER" -V "$RSM_PATH" -extract >"$INV"; then
	warn_or_fail "RSM signature verification failed"
fi

if ! sh "$ROOT/scripts/ci/tree-manifest.sh" verify-tracked "$INV"; then
	# Do not leave RNS_INVENTORY_OUT on failure. End-of-job
	# verify-workspace-clean treats a missing inventory as soft skip
	# CI always runs both checks as optional warn-only.
	warn_or_fail "tree inventory hash check failed"
fi

# Persist only after hashes match so optional failures soft-skip the recheck.
if [ -n "${RNS_INVENTORY_OUT:-}" ]; then
	cp "$INV" "$RNS_INVENTORY_OUT"
fi
echo "verify-tree-rsm.sh: OK (signer $SIGNER)"
