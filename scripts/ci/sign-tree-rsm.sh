#!/bin/sh
# Sign a byte-level tree inventory into reticulum-go.rsm (rnid-compatible).
#
# Requires a private identity file (64-byte .rid / rngit client_identity).
# Never commit the identity file.
#
# Env:
#   RNS_ID_PATH   path to identity (required unless -i given)
#   RNS_RSM_PATH  output path (default: reticulum-go.rsm in repo root)
#
# Usage:
#   RNS_ID_PATH=~/.local/share/reticulum-go/reticulum-go-release.rid sh scripts/ci/sign-tree-rsm.sh
#   sh scripts/ci/sign-tree-rsm.sh -i /path/to.rid
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

ID_PATH="${RNS_ID_PATH:-}"
RSM_PATH="${RNS_RSM_PATH:-$ROOT/reticulum-go.rsm}"
BIN="${RNS_ID_BIN:-$ROOT/bin/reticulum-go}"

while [ "$#" -gt 0 ]; do
	case "$1" in
	-i)
		ID_PATH="${2:?}"
		shift 2
		;;
	-o)
		RSM_PATH="${2:?}"
		shift 2
		;;
	*)
		echo "sign-tree-rsm.sh: unknown arg: $1" >&2
		exit 2
		;;
	esac
done

if [ -z "$ID_PATH" ]; then
	echo "sign-tree-rsm.sh: set RNS_ID_PATH or pass -i /path/to.rid" >&2
	exit 1
fi
if [ ! -f "$ID_PATH" ]; then
	echo "sign-tree-rsm.sh: identity not found: $ID_PATH" >&2
	exit 1
fi

if [ ! -x "$BIN" ]; then
	echo "sign-tree-rsm.sh: building reticulum-go..."
	(cd "$ROOT" && go build -o "$BIN" ./cmd/reticulum-go)
fi

INV="$(mktemp "${TMPDIR:-/tmp}/tree-inv.XXXXXX")"
trap 'rm -f "$INV"' EXIT INT

sh "$ROOT/scripts/ci/tree-manifest.sh" generate >"$INV"
"$BIN" id -i "$ID_PATH" -S "@$INV" -w "$RSM_PATH" -f
echo "sign-tree-rsm.sh: wrote $RSM_PATH"
