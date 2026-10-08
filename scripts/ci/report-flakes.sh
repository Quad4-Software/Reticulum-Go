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

# A test that passed on retry produces both a "--- FAIL" log line (first
# attempt) and a "testsummary: FLAKE <pkg> <Test> (passed on retry N)" line.
# Subtract retried-passed names from the raw FAIL set so a recovered test is
# reported only as flaky, not also as a hard failure.
grep -rhoE -- '--- FAIL: [A-Za-z_][A-Za-z0-9_]*' "$TMP/logs" |
	sed 's/--- FAIL: //' | sort -u >"$TMP/failed.txt" || true
grep -rhoE 'testsummary: FLAKE [A-Za-z0-9_./-]+ [A-Za-z_][A-Za-z0-9_]*' "$TMP/logs" |
	sed 's/.*testsummary: FLAKE //' | sort -u >"$TMP/flaked.txt" || true
awk '{print $NF}' "$TMP/flaked.txt" | sort -u >"$TMP/flaked_names.txt"
comm -23 "$TMP/failed.txt" "$TMP/flaked_names.txt" >"$TMP/hardfailed.txt" || true

if [ ! -s "$TMP/hardfailed.txt" ] && [ ! -s "$TMP/flaked.txt" ]; then
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

# Names filed this run. The OPEN issue list is fetched once before any
# creates, so a name seen again under a different kind would otherwise open
# a duplicate issue seconds later.
: >"$TMP/seen.txt"

report() {
	name="$1"
	kind="$2"
	if grep -Fqx "$name" "$TMP/seen.txt"; then
		return
	fi
	printf '%s\n' "$name" >>"$TMP/seen.txt"
	existing="$(printf '%s\n' "$OPEN" | grep -F "Flake: $name" | head -n1 | cut -f1 || true)"
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
done <"$TMP/hardfailed.txt"

while IFS= read -r t; do
	[ -n "$t" ] && report "${t##* }" "flaky (passed on retry)"
done <"$TMP/flaked.txt"

# Auto-close: on a green run, an open flake issue whose test ran and passed
# outright is resolved. A test absent from the logs entirely is left alone;
# the suite may not have covered it this run.
if [ "${RUN_CONCLUSION:-}" = "success" ]; then
	printf '%s\n' "$OPEN" | while IFS="$(printf '\t')" read -r num title; do
		name="${title#Flake: }"
		case "$name" in
		"$title" | "") continue ;;
		esac
		if grep -Fqx "$name" "$TMP/failed.txt" || grep -Fqx "$name" "$TMP/flaked_names.txt"; then
			continue
		fi
		if grep -rqE -- "--- PASS: ${name}( |$)" "$TMP/logs"; then
			if gh issue close "$num" --repo "$REPO" \
				--comment "Passed in run ${RUN_URL}; closing." >/dev/null 2>&1; then
				echo "auto-closed #${num} (${name} passed)"
			fi
		fi
	done
fi
