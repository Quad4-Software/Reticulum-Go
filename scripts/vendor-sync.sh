#!/bin/sh
# Refresh go.mod replace paths and vendor/ trees from a local Reticulum-Go-Deps checkout.
#
# Usage: vendor-sync.sh [libs_root]
#   libs_root defaults to ../Reticulum-Go-Deps relative to this repository.
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
LIBS_ROOT="${1:-$(CDPATH='' cd -- "$ROOT/../Reticulum-Go-Deps" 2>/dev/null && pwd || true)}"

if [ -z "$LIBS_ROOT" ] || [ ! -d "$LIBS_ROOT" ]; then
	echo "vendor-sync.sh: libs root not found; pass path to Reticulum-Go-Deps" >&2
	exit 1
fi

relpath() {
	python3 -c 'import os.path, sys; print(os.path.relpath(sys.argv[1], sys.argv[2]))' "$1" "$2"
}

REL_LIBS="$(relpath "$LIBS_ROOT" "$ROOT")"
REL_WASM_LIBS="$(relpath "$LIBS_ROOT" "$ROOT/examples/wasm")"
REL_PS_LIBS="$(relpath "$LIBS_ROOT" "$ROOT/examples/pageserver")"

PROTO_DIR=""
if [ -d "$LIBS_ROOT/reticulum-go-protocols" ]; then
	PROTO_DIR="reticulum-go-protocols"
elif [ -d "$LIBS_ROOT/reticulum-go-mf" ]; then
	PROTO_DIR="reticulum-go-mf"
fi

for lib in bzip2 msgpack pbt tagparser; do
	if [ ! -d "$LIBS_ROOT/$lib" ]; then
		echo "vendor-sync.sh: missing $LIBS_ROOT/$lib" >&2
		exit 1
	fi
done
if [ -z "$PROTO_DIR" ]; then
	echo "vendor-sync.sh: missing $LIBS_ROOT/reticulum-go-protocols (or reticulum-go-mf)" >&2
	exit 1
fi

cat > "$ROOT/go.mod" <<EOF
module github.com/Quad4-Software/Reticulum-Go

go 1.27.1

require (
	github.com/Quad4-Software/bzip2 v1.0.1
	github.com/Quad4-Software/msgpack/v5 v5.9.1
	github.com/Quad4-Software/pbt v1.0.2
	golang.org/x/crypto v0.57.0
	golang.org/x/sys v0.48.0
)

require github.com/Quad4-Software/tagparser/v2 v2.2.1 // indirect

replace (
	github.com/Quad4-Software/bzip2 => $REL_LIBS/bzip2
	github.com/Quad4-Software/msgpack/v5 => $REL_LIBS/msgpack
	github.com/Quad4-Software/pbt => $REL_LIBS/pbt
	github.com/Quad4-Software/tagparser/v2 => $REL_LIBS/tagparser
)
EOF

cat > "$ROOT/examples/wasm/go.mod" <<EOF
module github.com/Quad4-Software/Reticulum-Go/examples/wasm

go 1.27.1

require (
	github.com/Quad4-Software/Reticulum-Go v0.0.0
	github.com/Quad4-Software/reticulum-go-protocols v0.0.0
)

require (
	github.com/Quad4-Software/msgpack/v5 v5.9.1 // indirect
	github.com/Quad4-Software/pbt v1.0.2 // indirect
	github.com/Quad4-Software/tagparser/v2 v2.2.1 // indirect
	golang.org/x/crypto v0.57.0 // indirect
)

replace (
	github.com/Quad4-Software/Reticulum-Go => ../../
	github.com/Quad4-Software/bzip2 => $REL_WASM_LIBS/bzip2
	github.com/Quad4-Software/msgpack/v5 => $REL_WASM_LIBS/msgpack
	github.com/Quad4-Software/pbt => $REL_WASM_LIBS/pbt
	github.com/Quad4-Software/reticulum-go-protocols => $REL_WASM_LIBS/$PROTO_DIR
	github.com/Quad4-Software/tagparser/v2 => $REL_WASM_LIBS/tagparser
)
EOF

cat > "$ROOT/examples/pageserver/go.mod" <<EOF
module github.com/Quad4-Software/Reticulum-Go/examples/pageserver

go 1.27.1

require github.com/Quad4-Software/Reticulum-Go v0.0.0

require (
	github.com/Quad4-Software/bzip2 v1.0.1 // indirect
	github.com/Quad4-Software/msgpack/v5 v5.9.1 // indirect
	github.com/Quad4-Software/pbt v1.0.2 // indirect
	github.com/Quad4-Software/tagparser/v2 v2.2.1 // indirect
	golang.org/x/crypto v0.57.0 // indirect
)

replace (
	github.com/Quad4-Software/Reticulum-Go => ../..
	github.com/Quad4-Software/bzip2 => $REL_PS_LIBS/bzip2
	github.com/Quad4-Software/msgpack/v5 => $REL_PS_LIBS/msgpack
	github.com/Quad4-Software/pbt => $REL_PS_LIBS/pbt
	github.com/Quad4-Software/tagparser/v2 => $REL_PS_LIBS/tagparser
)
EOF

vendor_tree() {
	dir="$1"
	(
		cd "$dir"
		rm -rf vendor
		env GOWORK=off GOFLAGS= go mod tidy
		env GOWORK=off GOFLAGS= go mod vendor
	)
}

vendor_tree "$ROOT"
vendor_tree "$ROOT/examples/wasm"
vendor_tree "$ROOT/examples/pageserver"

# go mod vendor deletes local vendor patches on regeneration. Reapply them.
# Keep this list in step with the module roots vendored above.
sh "$ROOT/scripts/vendor-patches.sh" "$ROOT" "$ROOT/examples/wasm" "$ROOT/examples/pageserver"

echo "vendor-sync: vendor/ trees refreshed from $LIBS_ROOT"
