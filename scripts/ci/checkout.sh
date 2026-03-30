#!/bin/sh
# Clone or shallow-fetch the repository using Gitea/GitHub Actions-compatible env.
#
# Usage: checkout.sh [fetch_depth]
#   fetch_depth: commit depth (default 1), or 0 for full clone then checkout SHA.
#
# Env: GITEA_SERVER_URL or GITHUB_SERVER_URL, GITEA_REPOSITORY or GITHUB_REPOSITORY,
#      GITHUB_SHA. Optional: GITEA_TOKEN or GITHUB_TOKEN, GITEA_WORKSPACE or GITHUB_WORKSPACE.
set -eu

FETCH_DEPTH="${1:-1}"
SERVER="${GITEA_SERVER_URL:-${GITHUB_SERVER_URL:-}}"
REPO="${GITEA_REPOSITORY:-${GITHUB_REPOSITORY:-}}"
SHA="${GITHUB_SHA:-}"
TOKEN="${GITEA_TOKEN:-${GITHUB_TOKEN:-}}"
WORKSPACE="${GITEA_WORKSPACE:-${GITHUB_WORKSPACE:-.}}"

if [ -z "$SERVER" ] || [ -z "$REPO" ] || [ -z "$SHA" ]; then
    echo "checkout.sh: need SERVER/REPO/SHA (GITEA_* or GITHUB_* and GITHUB_SHA)" >&2
    exit 1
fi

cd "$WORKSPACE"

if [ -n "$TOKEN" ]; then
    git config --global credential.helper \
        "!f() { echo username=x-access-token; echo \"password=${TOKEN}\"; }; f"
fi

ORIGIN="${SERVER}/${REPO}.git"

if [ "$FETCH_DEPTH" = "0" ]; then
    git clone -q "$ORIGIN" .
else
    git init -q
    git remote add origin "$ORIGIN"
    git fetch -q --depth="$FETCH_DEPTH" origin "$SHA"
fi

git checkout -q "$SHA" 2>/dev/null || git checkout -q FETCH_HEAD

echo "Checked out ${REPO} at $(git rev-parse --short HEAD)"
