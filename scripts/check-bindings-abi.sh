#!/bin/sh
set -eu

usage() {
    echo "usage: sh scripts/check-bindings-abi.sh"
    echo "checks odin zig cpp rust dart python lua swift java; kotlin and android are excluded because kotlin wraps java"
}

case "${1:-}" in
    -h|--help) usage; exit 0 ;;
    "") ;;
    *) usage >&2; exit 2 ;;
esac

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
header="$root/include/rns.h"
version=$(sed -n 's/^#define RNS_API_VERSION "\([^"]*\)".*/\1/p' "$header")
if [ -z "$version" ]; then
    echo "could not read RNS_API_VERSION from include/rns.h" >&2
    exit 1
fi

funcs=$(grep -o 'rns_[a-z0-9_]*(' "$header" | sed 's/($//;s/(//' | sort -u)
langs="odin zig cpp rust dart python lua swift java"
failed=0

for lang in $langs; do
    dir="$root/bindings/$lang"
    missing=""
    for fn in $funcs; do
        if ! grep -R -q \
            --exclude-dir=examples --exclude-dir=example \
            --exclude-dir=tests --exclude-dir=test \
            --exclude-dir=out --exclude-dir=build \
            --exclude-dir=.zig-cache --exclude-dir=target --exclude-dir=.dart_tool \
            --include='*.odin' --include='*.zig' --include='*.cpp' --include='*.hpp' \
            --include='*.rs' --include='*.dart' --include='*.py' --include='*.lua' \
            --include='*.swift' --include='*.java' \
            "$fn" "$dir" 2>/dev/null; then
            missing="$missing $fn"
        fi
    done
    if [ -n "$missing" ]; then
        echo "$lang missing:$missing" >&2
        failed=1
    fi
    if ! grep -R -q \
        --exclude-dir=examples --exclude-dir=example \
        --exclude-dir=tests --exclude-dir=test \
        --exclude-dir=out --exclude-dir=build \
        --exclude-dir=.zig-cache --exclude-dir=target --exclude-dir=.dart_tool \
        --include='*.odin' --include='*.zig' --include='*.cpp' --include='*.hpp' \
        --include='*.rs' --include='*.dart' --include='*.py' --include='*.lua' \
        --include='*.swift' --include='*.java' \
        -e "$version" -e 'RNS_API_VERSION' "$dir" 2>/dev/null; then
        echo "$lang missing API version $version" >&2
        failed=1
    fi
done

if [ "$failed" -ne 0 ]; then
    exit 1
fi
echo "bindings ABI check passed"
