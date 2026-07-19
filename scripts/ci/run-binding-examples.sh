#!/bin/sh
# Build (and run smoke where applicable) for one language binding's examples.
# Usage: run-binding-examples.sh <c|odin|zig|cpp|dart|rust|python|lua|swift|java|kotlin>
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

lang="${1:-}"
if [ -z "$lang" ]; then
	echo "usage: $0 <c|odin|zig|cpp|dart|rust|python|lua|swift|java|kotlin>" >&2
	exit 2
fi

case "$lang" in
c | odin | zig | cpp | dart | rust | python | lua | swift | java | kotlin) ;;
*)
	echo "unknown binding language: $lang" >&2
	exit 2
	;;
esac

if [ ! -f bin/librns.so ]; then
	task build-librns
fi

export LIBRARY_PATH="${ROOT}/bin:${LIBRARY_PATH:-}"
export LD_LIBRARY_PATH="${ROOT}/bin:${LD_LIBRARY_PATH:-}"
export RNS_LIB_DIR="${ROOT}/bin"
export RNS_LIB_PATH="${ROOT}/bin/librns.so"
export RNS_ROOT="${ROOT}"

make -C "bindings/${lang}" examples
