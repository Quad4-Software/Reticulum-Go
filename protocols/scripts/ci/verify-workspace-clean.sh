#!/bin/sh
# Fail on unexpected git working tree changes after CI steps.
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

ALLOW_PREFIXES="
bin/
coverage.out
LXMF-ref/
RRC-ref/
.venv/
.cache/
.gotmp/
"

is_allowed() {
	p="$1"
	for a in $ALLOW_PREFIXES; do
		case "$p" in
		"$a" | "$a"*) return 0 ;;
		esac
	done
	case "$p" in
	vendor | vendor/*) return 0 ;;
	*.test | *.log | *.out) return 0 ;;
	esac
	return 1
}

fail=0
tmp="$(mktemp "${TMPDIR:-/tmp}/mf-clean.XXXXXX")"
trap 'rm -f "$tmp"' EXIT INT
git status --porcelain -u >"$tmp"
while IFS= read -r line; do
	[ -z "$line" ] && continue
	xy="$(printf '%s\n' "$line" | cut -c1-2)"
	path="$(printf '%s\n' "$line" | sed 's/^.. //;s/.* -> //')"
	case "$xy" in
	"??")
		if is_allowed "$path"; then continue; fi
		echo "verify-workspace-clean: unexpected untracked: $path" >&2
		fail=1
		;;
	*)
		if is_allowed "$path"; then continue; fi
		echo "verify-workspace-clean: unexpected change: $line" >&2
		fail=1
		;;
	esac
done <"$tmp"

if [ "$fail" -ne 0 ]; then
	exit 1
fi
echo "verify-workspace-clean: OK"
