#!/bin/sh
# Staged Go checks for pre-commit: gofmt and go vet on affected packages.
#
# Skip: SKIP_GO_HOOK=1 or SKIP_LINT_HOOK=1
set -eu

if [ "${SKIP_LINT_HOOK:-0}" = "1" ] || [ "${SKIP_GO_HOOK:-0}" = "1" ]; then
	exit 0
fi

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

STAGED="$(git diff --cached --name-only --diff-filter=ACMR -- '*.go' ':!:vendor/**' ':!:**/vendor/**')"
if [ -z "$STAGED" ]; then
	exit 0
fi

# Files under nested modules (examples/*, testdata) are outside the main
# module; package patterns for them fail go vet. gofmt still checks them.
VET_STAGED="$STAGED"
NESTED_MOD_DIRS="$(git ls-files | grep 'go\.mod$' | grep -v '^go\.mod$' | grep -v '^vendor/' | grep -v '/vendor/' | xargs -r -n1 dirname | sort -u)"
if [ -n "$NESTED_MOD_DIRS" ]; then
	VET_STAGED=""
	while IFS= read -r path; do
		[ -n "$path" ] || continue
		nested=0
		while IFS= read -r moddir; do
			case "$path" in
			"$moddir"/*) nested=1; break ;;
			esac
		done <<EOF
$NESTED_MOD_DIRS
EOF
		if [ "$nested" -eq 0 ]; then
			VET_STAGED="$VET_STAGED$path
"
		fi
	done <<EOF
$STAGED
EOF
fi

if ! command -v go >/dev/null 2>&1; then
	echo "pre-commit: go not found" >&2
	exit 1
fi

UNFORMATTED="$(gofmt -l $STAGED)"
if [ -n "$UNFORMATTED" ]; then
	echo "pre-commit: Go files need formatting (run task fmt):" >&2
	echo "$UNFORMATTED" >&2
	exit 1
fi

pkgs=
while IFS= read -r path; do
	[ -n "$path" ] || continue
	[ -f "$path" ] || continue
	dir="$(dirname "$path")"
	pkg="./${dir%/}/..."
	pkgs="$pkgs $pkg"
done <<EOF
$VET_STAGED
EOF

# shellcheck disable=SC2086
set -- $pkgs
if [ "$#" -eq 0 ]; then
	exit 0
fi

unique_pkgs="$(printf '%s\n' "$@" | sort -u)"
echo "pre-commit: go vet ($# package path(s))"
# shellcheck disable=SC2086
go vet $unique_pkgs
