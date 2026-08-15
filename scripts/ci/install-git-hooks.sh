#!/bin/sh
# Point this clone at .githooks/ (tracked git hooks).
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

git config core.hooksPath .githooks
chmod +x .githooks/pre-commit scripts/ci/pre-commit-lint.sh
echo "install-git-hooks.sh: core.hooksPath=.githooks"
