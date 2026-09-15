#!/bin/sh
# Refresh go.mod replace paths and vendor/ trees from a local Reticulum-Go-Projects checkout.
#
# Usage: vendor-sync.sh [libs_root]
#   libs_root defaults to ../../Reticulum-Go-Projects relative to this repository.
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
LIBS_ROOT="${1:-$(CDPATH='' cd -- "$ROOT/../../Reticulum-Go-Projects" 2>/dev/null && pwd || true)}"

if [ -z "$LIBS_ROOT" ] || [ ! -d "$LIBS_ROOT" ]; then
	echo "vendor-sync.sh: libs root not found; pass path to Reticulum-Go-Projects" >&2
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
	github.com/Quad4-Software/bzip2 v0.0.0
	github.com/Quad4-Software/msgpack/v5 v5.8.1
	github.com/Quad4-Software/pbt v0.0.0
	golang.org/x/crypto v0.52.0
	golang.org/x/sys v0.45.0
)

require github.com/Quad4-Software/tagparser v0.0.0 // indirect

replace (
	github.com/Quad4-Software/bzip2 => $REL_LIBS/bzip2
	github.com/Quad4-Software/msgpack/v5 => $REL_LIBS/msgpack
	github.com/Quad4-Software/pbt => $REL_LIBS/pbt
	github.com/Quad4-Software/tagparser => $REL_LIBS/tagparser
	quad4/reticulum-go-protocols => $REL_LIBS/$PROTO_DIR
)
EOF

cat > "$ROOT/examples/wasm/go.mod" <<EOF
module github.com/Quad4-Software/Reticulum-Go/examples/wasm

go 1.27.1

require (
	github.com/Quad4-Software/Reticulum-Go v0.0.0
	quad4/reticulum-go-protocols v0.0.0
)

require (
	github.com/Quad4-Software/msgpack/v5 v5.8.1 // indirect
	github.com/Quad4-Software/pbt v0.0.0 // indirect
	github.com/Quad4-Software/tagparser v0.0.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
)

replace (
	github.com/Quad4-Software/Reticulum-Go => ../../
	github.com/Quad4-Software/bzip2 => $REL_WASM_LIBS/bzip2
	github.com/Quad4-Software/msgpack/v5 => $REL_WASM_LIBS/msgpack
	github.com/Quad4-Software/pbt => $REL_WASM_LIBS/pbt
	github.com/Quad4-Software/tagparser => $REL_WASM_LIBS/tagparser
	quad4/reticulum-go-protocols => $REL_WASM_LIBS/$PROTO_DIR
	// quad4/* names below satisfy reticulum-go-protocols' own requires, which
	// still use the old module paths upstream.
	quad4/bzip2 => github.com/Quad4-Software/bzip2 v0.0.0-20260704225916-ca8b2bb66059
	quad4/msgpack/v5 => github.com/Quad4-Software/msgpack/v5 v5.8.2
	quad4/pbt => github.com/Quad4-Software/pbt v0.0.0-20260614183135-abe0cfc4e604
	quad4/reticulum-go => github.com/Quad4-Software/Reticulum-Go v1.1.1
	quad4/tagparser => github.com/Quad4-Software/tagparser v0.1.3-0.20260614183136-daa4d5f437ce
)
EOF

cat > "$ROOT/examples/pageserver/go.mod" <<EOF
module github.com/Quad4-Software/Reticulum-Go/examples/pageserver

go 1.27.1

require github.com/Quad4-Software/Reticulum-Go v0.0.0

require (
	github.com/Quad4-Software/bzip2 v0.0.0 // indirect
	github.com/Quad4-Software/msgpack/v5 v5.8.1 // indirect
	github.com/Quad4-Software/pbt v0.0.0 // indirect
	github.com/Quad4-Software/tagparser v0.0.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
)

replace (
	github.com/Quad4-Software/Reticulum-Go => ../..
	github.com/Quad4-Software/bzip2 => $REL_PS_LIBS/bzip2
	github.com/Quad4-Software/msgpack/v5 => $REL_PS_LIBS/msgpack
	github.com/Quad4-Software/pbt => $REL_PS_LIBS/pbt
	github.com/Quad4-Software/tagparser => $REL_PS_LIBS/tagparser
)
EOF

vendor_tree() {
	dir="$1"
	(
		cd "$dir"
		rm -rf vendor
		env GOWORK=off GOFLAGS= GOPROXY=off GOSUMDB=off go mod tidy
		env GOWORK=off GOFLAGS= GOPROXY=off GOSUMDB=off go mod vendor
	)
}

vendor_tree "$ROOT"
vendor_tree "$ROOT/examples/wasm"
vendor_tree "$ROOT/examples/pageserver"

# Flip the main module's replace directives to the committed in-repo vendor
# layout and drop go.mod files into the vendored trees so the dir replaces
# resolve in -mod=mod mode as well.
VENDOR_BASE="./vendor/github.com/Quad4-Software"
for pair in "bzip2:bzip2" "msgpack:msgpack/v5" "pbt:pbt" "tagparser:tagparser"; do
	repo="${pair%%:*}"
	modpath="${pair##*:}"
	sed -i "s|github.com/Quad4-Software/$modpath => $REL_LIBS/$repo|github.com/Quad4-Software/$modpath => $VENDOR_BASE/$modpath|" "$ROOT/go.mod" "$ROOT/vendor/modules.txt"
	cp "$LIBS_ROOT/$repo/go.mod" "$ROOT/vendor/github.com/Quad4-Software/$modpath/go.mod"
done

echo "vendor-sync: vendor/ trees refreshed from $LIBS_ROOT"
