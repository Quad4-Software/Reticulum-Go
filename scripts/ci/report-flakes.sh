#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum
# Copyright (c) 2024-2026 Quad4.io
#
# Report CI test flakes as GitHub issues. Intended to run from the
# flake-report workflow (workflow_run trigger) with GH_TOKEN and RUN_ID set.
# Collects tests that failed outright (--- FAIL:) and tests that only passed
# after a testsummary retry (testsummary: FLAKE), then opens or updates a
# ci-flake issue per test.
set -eu

: "${RUN_ID:?RUN_ID required}"
: "${GH_TOKEN:?GH_TOKEN required}"
REPO="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY required}"

LABEL="ci-flake"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if ! gh api "repos/${REPO}/actions/runs/${RUN_ID}/logs" >"$TMP/logs.zip" 2>/dev/null; then
	echo "no logs for run ${RUN_ID}"
	exit 0
fi
unzip -qo "$TMP/logs.zip" -d "$TMP/logs" 2>/dev/null || {
	echo "could not unzip logs for run ${RUN_ID}"
	exit 0
}

# Failed tests look like "--- FAIL: TestFoo (1.23s)". Retried flakes look
# like "testsummary: FLAKE <pkg> <Test> (passed on retry N)".
grep -rhoE -- '--- FAIL: [A-Za-z_][A-Za-z0-9_]*' "$TMP/logs" |
	sed 's/--- FAIL: //' | sort -u >"$TMP/failed.txt" || true
grep -rhoE 'testsummary: FLAKE [^ ]+ [A-Za-z_][A-Za-z0-9_]*' "$TMP/logs" |
	sed 's/.*testsummary: FLAKE //' | sort -u >"$TMP/flaked.txt" || true

if [ ! -s "$TMP/failed.txt" ] && [ ! -s "$TMP/flaked.txt" ]; then
	echo "no failed or flaky tests in run ${RUN_ID}"
	exit 0
fi

gh label create "$LABEL" \
	--repo "$REPO" \
	--description "Automated report of a flaky CI test" \
	--color "f9d0c4" 2>/dev/null || true

RUN_URL="https://github.com/${REPO}/actions/runs/${RUN_ID}"
OPEN="$(gh issue list --repo "$REPO" --label "$LABEL" --state open --limit 200 \
	--json number,title --jq '.[] | "\(.number)\t\(.title)"' 2>/dev/null || true)"

report() {
	name="$1"
	kind="$2"
	existing="$(printf '%s\n' "$OPEN" | grep -F "$name" | head -n1 | cut -f1 || true)"
	if [ -n "$existing" ]; then
		gh issue comment "$existing" --repo "$REPO" \
			--body "Seen again in run ${RUN_URL} (${kind})." >/dev/null
		echo "commented on #${existing} for ${name}"
	else
		gh issue create --repo "$REPO" --label "$LABEL" \
			--title "Flake: ${name}" \
			--body "Test \`${name}\` reported as ${kind} in CI run ${RUN_URL}." >/dev/null
		echo "opened issue for ${name}"
	fi
}

# Flake lines carry "pkg Test"; reduce to the bare test name so dedup keys
# match the "--- FAIL:" entries.
while IFS= read -r t; do
	[ -n "$t" ] && report "$t" "a failure"
done <"$TMP/failed.txt"

while IFS= read -r t; do
	[ -n "$t" ] && report "${t##* }" "flaky (passed on retry)"
done <"$TMP/flaked.txt"
