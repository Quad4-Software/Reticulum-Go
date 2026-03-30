#!/bin/sh
# Create a Python venv in the workspace and pip install packages; expose bin on PATH for later steps.
# Usage: setup-venv-pip.sh <pip_package> [more packages...]
set -eu

WS="${GITEA_WORKSPACE:-${GITHUB_WORKSPACE:-.}}"
cd "$WS" || exit 1

python3 -m venv .venv
. .venv/bin/activate
python3 -m pip install --upgrade pip
python3 -m pip install "$@"
deactivate

if [ -n "${GITHUB_PATH:-}" ]; then
    echo "$WS/.venv/bin" >> "$GITHUB_PATH"
fi
if [ -n "${GITEA_PATH:-}" ]; then
    echo "$WS/.venv/bin" >> "$GITEA_PATH"
fi
