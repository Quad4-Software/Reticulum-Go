#!/usr/bin/env bash
# Portable CI setup (no GitHub Actions).
set -euo pipefail

. "$(dirname "$0")/env.sh"

if ! ci_truthy "${CI_SKIP_BOOTSTRAP:-}"; then
	if ci_truthy "${CI_BOOTSTRAP_INSTALL:-}"; then
		bash "$ROOT/scripts/ci/bootstrap-toolchain.sh" install
	else
		bash "$ROOT/scripts/ci/bootstrap-toolchain.sh" verify || bash "$ROOT/scripts/ci/bootstrap-toolchain.sh" install
	fi
fi

if ! ci_truthy "${CI_SKIP_TREE_VERIFY:-}"; then
	export RNS_INVENTORY_OUT="${RNS_INVENTORY_OUT:-${TMPDIR:-/tmp}/reticulum-go-ci-inventory.$$}"
	sh "$ROOT/scripts/ci/verify-tree-rsm.sh"
fi

echo "ci setup: OK"
