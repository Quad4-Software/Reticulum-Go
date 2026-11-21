#!/bin/sh
# Install pinned Go, Task, and optional CI tools.
# Env: CI_GO_VERSION, CI_TASK_VERSION, CI_REVIVE_VERSION, CI_GOSEC_VERSION
# Set CI_INSTALL_REVIVE=1 and/or CI_INSTALL_GOSEC=1 to install linters.
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

GO_VER="${CI_GO_VERSION:?CI_GO_VERSION required}"
TASK_VER="${CI_TASK_VERSION:?CI_TASK_VERSION required}"

sh "$ROOT/scripts/ci/setup-go.sh" "$GO_VER"
sh "$ROOT/scripts/ci/setup-task.sh" "$TASK_VER"

if [ "${CI_INSTALL_REVIVE:-0}" = "1" ] && [ -n "${CI_REVIVE_VERSION:-}" ]; then
	sh "$ROOT/scripts/ci/setup-revive.sh" "$CI_REVIVE_VERSION"
fi
if [ "${CI_INSTALL_GOSEC:-0}" = "1" ] && [ -n "${CI_GOSEC_VERSION:-}" ]; then
	sh "$ROOT/scripts/ci/setup-gosec.sh" "$CI_GOSEC_VERSION"
fi

if [ "${CI_INSTALL_APT_DEPS:-1}" = "1" ]; then
	sh "$ROOT/scripts/ci/setup-ci-apt-deps.sh"
fi

export PATH="/usr/local/go/bin:${PATH}"
go version
