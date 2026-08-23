#!/bin/sh
# Point this clone at .githooks/ (tracked git hooks).
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

git config core.hooksPath .githooks
chmod +x .githooks/pre-commit .githooks/commit-msg .githooks/pre-push
chmod +x scripts/ci/pre-commit-lint.sh scripts/ci/pre-commit-go.sh scripts/ci/commit-msg-check.sh
echo "install-git-hooks.sh: core.hooksPath=.githooks"
