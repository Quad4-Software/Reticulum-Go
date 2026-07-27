#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
RETICULUM_REF="$ROOT_DIR/reticulum-ref"
REPO_URL_RNS="rns://7649a50d84610232d1416b41d2896aff/reticulum/reticulum"
REPO_URL_GITHUB="https://github.com/markqvist/Reticulum.git"
# Wire-compat target for crossref vectors and interop.
RNS_REF_TAG="${RNS_REF_TAG:-1.4.2}"

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
	echo "Cloning Python Reticulum $RNS_REF_TAG from $url..."
	git clone --depth 1 --branch "$RNS_REF_TAG" "$url" "$RETICULUM_REF"
}

checkout_ref_tag() {
	(
		cd "$RETICULUM_REF"
		git fetch --tags --force origin "refs/tags/${RNS_REF_TAG}:refs/tags/${RNS_REF_TAG}" 2>/dev/null \
			|| git fetch --depth 1 origin tag "$RNS_REF_TAG"
		git checkout -q "tags/${RNS_REF_TAG}" 2>/dev/null || git checkout -q "$RNS_REF_TAG"
		git reset --hard "tags/${RNS_REF_TAG}" 2>/dev/null || git reset --hard "$RNS_REF_TAG"
	)
}

clone_or_update() {
	if [ -d "$RETICULUM_REF/RNS" ]; then
		current_url="$(cd "$RETICULUM_REF" && git remote get-url origin 2>/dev/null || true)"
		if [ -n "$current_url" ] && [ "$current_url" != "$REPO_URL_RNS" ] && [ "$current_url" != "$REPO_URL_GITHUB" ]; then
			echo "reticulum-ref remote changed; re-cloning..."
			rm -rf "$RETICULUM_REF"
		else
			echo "Updating reticulum-ref to $RNS_REF_TAG..."
			if checkout_ref_tag; then
				return 0
			fi
			echo "tag checkout failed; re-cloning..."
			rm -rf "$RETICULUM_REF"
		fi
	elif [ -d "$RETICULUM_REF" ]; then
		rm -rf "$RETICULUM_REF"
	fi

	if git clone --depth 1 --branch "$RNS_REF_TAG" "$REPO_URL_RNS" "$RETICULUM_REF" 2>/dev/null; then
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
	go run ./scripts/ci/testsummary -v ./tests/crossref/
}

case "${1:-all}" in
	generate)
		generate_vectors
		;;
	test)
		run_tests
		;;
	diff)
		generate_vectors
		run_tests
		;;
	all)
		generate_vectors
		run_tests
		;;
	*)
		echo "Usage: $0 {generate|test|diff|all}"
		exit 1
		;;
esac
