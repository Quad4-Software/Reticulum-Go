#!/usr/bin/env bash
# Run CI jobs outside GitHub Actions (local shell, Forgejo, Gitea).
#
# Usage:
#   bash scripts/ci/run.sh <job>
#
# Jobs:
#   pr         task check (fmt, vet, lint, staticcheck, tests, scan, vulncheck)
#   lint       task ci (fmt-check, vet, lint, staticcheck)
#   prepush    task prepush
#   list       Print job names
#
# Env:
#   CI_SKIP_SETUP=1        Skip setup.sh
#   CI_SKIP_TREE_VERIFY=1  Skip reticulum-go.rsm verify
#   RNS_INVENTORY_OUT      End-of-job workspace clean check
set -euo pipefail

. "$(dirname "$0")/env.sh"

run_setup() {
	if ci_truthy "${CI_SKIP_SETUP:-}"; then
		return 0
	fi
	bash "$ROOT/scripts/ci/setup.sh"
}

job_pr() {
	run_setup
	require_task
	task check
	verify_workspace_clean
}

job_lint() {
	run_setup
	require_task
	task ci
	verify_workspace_clean
}

job_prepush() {
	run_setup
	require_task
	task prepush
	verify_workspace_clean
}

usage() {
	sed -n '6,16p' "$0" | sed 's/^# \?//'
}

job="${1:-pr}"
case "$job" in
list)
	echo "pr lint prepush"
	;;
pr) job_pr ;;
lint) job_lint ;;
prepush) job_prepush ;;
-h | --help | help)
	usage
	;;
*)
	echo "ci: unknown job: $job" >&2
	usage >&2
	exit 2
	;;
esac
