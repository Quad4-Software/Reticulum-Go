#!/bin/sh
# Verify local dev tools against scripts/ci/dev-tools.env pins.
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

. "$ROOT/scripts/ci/dev-tools.env"

warn=0
fail=0

version_match() {
	label="$1"
	got="$2"
	want="$3"
	if [ -z "$got" ]; then
		echo "MISSING: $label version" >&2
		fail=$((fail + 1))
		return
	fi
	case "$label" in
	Go)
		if echo "$got" | grep -q "$want"; then
			echo "OK: Go $got (want $want)"
		else
			echo "WARN: Go $got (want $want)" >&2
			warn=$((warn + 1))
		fi
		;;
	Task)
		if echo "$got" | grep -q "$want"; then
			echo "OK: Task $got (want $want)"
		else
			echo "WARN: Task $got (want $want)" >&2
			warn=$((warn + 1))
		fi
		;;
	*)
		if echo "$got" | grep -q "${want#v}"; then
			echo "OK: $label $got (want $want)"
		else
			echo "WARN: $label $got (want $want)" >&2
			warn=$((warn + 1))
		fi
		;;
	esac
}

echo "doctor: checking dev tools (pins in scripts/ci/dev-tools.env)"
echo ""

if command -v go >/dev/null 2>&1; then
	version_match Go "$(go version)" "$CI_GO_VERSION"
else
	echo "MISSING: go" >&2
	fail=$((fail + 1))
fi

if command -v task >/dev/null 2>&1; then
	version_match Task "$(task --version 2>/dev/null | head -1)" "$CI_TASK_VERSION"
else
	echo "MISSING: task (run task bootstrap)" >&2
	fail=$((fail + 1))
fi

if command -v revive >/dev/null 2>&1; then
	version_match revive "$(revive --version 2>/dev/null | head -1)" "$CI_REVIVE_VERSION"
else
	echo "MISSING: revive (run task bootstrap)" >&2
	fail=$((fail + 1))
fi

if command -v staticcheck >/dev/null 2>&1; then
	version_match staticcheck "$(staticcheck -version 2>/dev/null | head -1)" "$CI_STATICCHECK_VERSION"
else
	echo "MISSING: staticcheck (run task bootstrap)" >&2
	fail=$((fail + 1))
fi

if command -v shellcheck >/dev/null 2>&1; then
	echo "OK: shellcheck on PATH"
else
	echo "WARN: shellcheck not found (optional for hooks)" >&2
	warn=$((warn + 1))
fi

if command -v yamllint >/dev/null 2>&1; then
	echo "OK: yamllint on PATH"
else
	echo "WARN: yamllint not found (PyYAML fallback in hooks)" >&2
	warn=$((warn + 1))
fi

if command -v python3 >/dev/null 2>&1; then
	echo "OK: python3 on PATH"
else
	echo "WARN: python3 not found (optional for crossref vectors)" >&2
	warn=$((warn + 1))
fi

ID_PATH="${RNS_ID_PATH:-}"
if [ -n "$ID_PATH" ] && [ -f "$ID_PATH" ]; then
	echo "OK: RNS_ID_PATH=$ID_PATH"
else
	for candidate in \
		"$HOME/.local/share/reticulum-go/reticulum-go-release.rid" \
		"$HOME/.rngit/client_identity"
	do
		if [ -f "$candidate" ]; then
			echo "OK: signing identity $candidate"
			ID_PATH="$candidate"
			break
		fi
	done
	if [ -z "$ID_PATH" ]; then
		echo "WARN: no RSM signing identity (optional for forks)" >&2
		warn=$((warn + 1))
	fi
fi

HOOKS_PATH="$(git config --get core.hooksPath 2>/dev/null || true)"
if [ "$HOOKS_PATH" = ".githooks" ]; then
	echo "OK: core.hooksPath=.githooks"
else
	echo "WARN: hooks not installed (run task hooks:install)" >&2
	warn=$((warn + 1))
fi

echo ""
if [ "$fail" -gt 0 ]; then
	echo "doctor: $fail required tool(s) missing, $warn warning(s)" >&2
	exit 1
fi
if [ "$warn" -gt 0 ]; then
	echo "doctor: OK with $warn warning(s)"
else
	echo "doctor: all checks passed"
fi
