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

# The protocols module lives in this repository under protocols/.
PROTO_DIR="protocols"
REL_PROTO="$(relpath "$ROOT/protocols" "$ROOT/examples/wasm")"
if [ ! -d "$ROOT/protocols" ]; then
	echo "vendor-sync.sh: missing $ROOT/protocols" >&2
	exit 1
fi

for lib in bzip2 msgpack pbt tagparser; do
	if [ ! -d "$LIBS_ROOT/$lib" ]; then
		echo "vendor-sync.sh: missing $LIBS_ROOT/$lib" >&2
		exit 1
	fi
done

cat > "$ROOT/go.mod" <<EOF
module git.quad4.io/reticulum/Reticulum-Go

go 1.27.4

require (
	git.quad4.io/libraries/bzip2 v1.0.2
	git.quad4.io/libraries/msgpack/v5 v5.9.3
	git.quad4.io/libraries/pbt v1.0.3
	github.com/landlock-lsm/go-landlock v0.11.2
	golang.org/x/crypto v0.59.0
	golang.org/x/sys v0.50.0
)

require (
	git.quad4.io/libraries/tagparser/v2 v2.2.2 // indirect
	golang.org/x/net v0.62.0 // indirect
)

replace (
	git.quad4.io/libraries/bzip2 => $REL_LIBS/bzip2
	git.quad4.io/libraries/msgpack/v5 => $REL_LIBS/msgpack
	git.quad4.io/libraries/pbt => $REL_LIBS/pbt
	git.quad4.io/libraries/tagparser/v2 => $REL_LIBS/tagparser
)
EOF

cat > "$ROOT/examples/wasm/go.mod" <<EOF
module git.quad4.io/reticulum/Reticulum-Go/examples/wasm

go 1.27.4

require (
	git.quad4.io/reticulum/Reticulum-Go v0.0.0
	git.quad4.io/reticulum/Reticulum-Go/protocols v0.0.0
)

require (
	git.quad4.io/libraries/msgpack/v5 v5.9.3 // indirect
	git.quad4.io/libraries/pbt v1.0.3 // indirect
	git.quad4.io/libraries/tagparser/v2 v2.2.2 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/net v0.62.0 // indirect
)

replace (
	git.quad4.io/reticulum/Reticulum-Go => ../../
	git.quad4.io/libraries/bzip2 => $REL_WASM_LIBS/bzip2
	git.quad4.io/libraries/msgpack/v5 => $REL_WASM_LIBS/msgpack
	git.quad4.io/libraries/pbt => $REL_WASM_LIBS/pbt
	git.quad4.io/reticulum/Reticulum-Go/protocols => $REL_PROTO
	git.quad4.io/libraries/tagparser/v2 => $REL_WASM_LIBS/tagparser
)
EOF

cat > "$ROOT/examples/pageserver/go.mod" <<EOF
module git.quad4.io/reticulum/Reticulum-Go/examples/pageserver

go 1.27.4

require git.quad4.io/reticulum/Reticulum-Go v0.0.0

require (
	git.quad4.io/libraries/bzip2 v1.0.2 // indirect
	git.quad4.io/libraries/msgpack/v5 v5.9.3 // indirect
	git.quad4.io/libraries/pbt v1.0.3 // indirect
	git.quad4.io/libraries/tagparser/v2 v2.2.2 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/net v0.62.0 // indirect
)

replace (
	git.quad4.io/reticulum/Reticulum-Go => ../..
	git.quad4.io/libraries/bzip2 => $REL_PS_LIBS/bzip2
	git.quad4.io/libraries/msgpack/v5 => $REL_PS_LIBS/msgpack
	git.quad4.io/libraries/pbt => $REL_PS_LIBS/pbt
	git.quad4.io/libraries/tagparser/v2 => $REL_PS_LIBS/tagparser
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
