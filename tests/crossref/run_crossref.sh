#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
RETICULUM_REF="$ROOT_DIR/reticulum-ref"
REPO_URL_RNS="rns://7649a50d84610232d1416b41d2896aff/reticulum/reticulum"
REPO_URL_GITHUB="https://github.com/markqvist/Reticulum.git"

pip_reticulum_path() {
	python3 - <<'PY'
import os
import sys
try:
    import RNS
except ImportError:
    sys.exit(1)
root = os.path.dirname(os.path.dirname(os.path.abspath(RNS.__file__)))
if os.path.isdir(os.path.join(root, "RNS")):
    print(root)
PY
}

clone_repo() {
	local url="$1"
	echo "Cloning Python Reticulum from $url..."
	git clone --depth 1 "$url" "$RETICULUM_REF"
}

clone_or_update() {
	if [ -d "$RETICULUM_REF/RNS" ]; then
		current_url="$(cd "$RETICULUM_REF" && git remote get-url origin 2>/dev/null || true)"
		if [ -n "$current_url" ] && [ "$current_url" != "$REPO_URL_RNS" ] && [ "$current_url" != "$REPO_URL_GITHUB" ]; then
			echo "reticulum-ref remote changed; re-cloning..."
			rm -rf "$RETICULUM_REF"
		else
			echo "Updating reticulum-ref..."
			(cd "$RETICULUM_REF" && git fetch --depth 1 origin && git reset --hard FETCH_HEAD)
			return 0
		fi
	elif [ -d "$RETICULUM_REF" ]; then
		rm -rf "$RETICULUM_REF"
	fi

	if git clone --depth 1 "$REPO_URL_RNS" "$RETICULUM_REF" 2>/dev/null; then
		return 0
	fi

	echo "rngit clone failed; trying GitHub mirror..."
	if clone_repo "$REPO_URL_GITHUB"; then
		return 0
	fi

	pip_path="$(pip_reticulum_path || true)"
	if [ -n "$pip_path" ] && [ -d "$pip_path/RNS" ]; then
		export RETICULUM_PATH="$pip_path"
		echo "Using pip-installed RNS at $RETICULUM_PATH"
		return 0
	fi

	echo "Could not obtain Python Reticulum reference (rngit, GitHub, or pip)" >&2
	return 1
}

generate_vectors() {
	clone_or_update
	if [ -z "${RETICULUM_PATH:-}" ]; then
		export RETICULUM_PATH="$RETICULUM_REF"
	fi
	cd "$(dirname "$0")"
	python3 generate_vectors.py
	cd "$ROOT_DIR"
}

run_tests() {
	cd "$ROOT_DIR"
	go test -v ./tests/crossref/
}

case "${1:-all}" in
	generate)
		generate_vectors
		;;
	test)
		run_tests
		;;
	all)
		generate_vectors
		run_tests
		;;
	*)
		echo "Usage: $0 {generate|test|all}"
		exit 1
		;;
esac
