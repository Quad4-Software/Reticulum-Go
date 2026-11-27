#!/bin/sh
# Clone or shallow-fetch the repository using GitHub Actions-compatible env.
#
# CI workflows may use actions/checkout on GitHub or inline git in other hosts. This script
# is for local runs or act when you want a single shared implementation.
# Use this script for local runs or act when you want a single shared implementation.
#
# Usage: checkout.sh [fetch_depth]
#   fetch_depth: commit depth (default 1), or 0 for full clone then checkout SHA.
#
# Env: GITHUB_SERVER_URL, GITHUB_REPOSITORY,
#      GITHUB_SHA, optional GITHUB_REF / GITHUB_REF_NAME.
#      Optional: GITHUB_TOKEN, GITHUB_WORKSPACE.
#
# Prefers fetching GITHUB_REF when set (e.g. refs/heads/master) because some Git hosts
# do not allow shallow fetch by raw SHA over HTTPS.
set -eu

FETCH_DEPTH="${1:-1}"
SERVER="${GITHUB_SERVER_URL:-}"
REPO="${GITHUB_REPOSITORY:-}"
SHA="${GITHUB_SHA:-}"
REF="${GITHUB_REF:-}"
TOKEN="${GITHUB_TOKEN:-}"
WORKSPACE="${GITHUB_WORKSPACE:-.}"

REF_TYPE="${GITHUB_REF_TYPE:-branch}"
if [ -z "$REF" ] && [ -n "${GITHUB_REF_NAME:-}" ]; then
    case "$REF_TYPE" in
        tag)
            REF="refs/tags/${GITHUB_REF_NAME}"
            ;;
        *)
            REF="refs/heads/${GITHUB_REF_NAME}"
            ;;
    esac
fi

SERVER="${SERVER%/}"

if [ -z "$SERVER" ] || [ -z "$REPO" ]; then
    echo "checkout.sh: need SERVER and REPO (GITHUB_* URL and repository)" >&2
    exit 1
fi

if [ -z "$SHA" ] && [ -z "$REF" ]; then
    echo "checkout.sh: need GITHUB_SHA and/or GITHUB_REF (or GITHUB_REF_NAME)" >&2
    exit 1
fi

cd "$WORKSPACE"

if [ -n "$TOKEN" ]; then
    git config --global credential.helper \
        "!f() { echo username=x-access-token; echo \"password=${TOKEN}\"; }; f"
fi

ORIGIN="${SERVER}/${REPO}.git"

checkout_target() {
    if [ -n "$SHA" ]; then
        git checkout -q "$SHA" 2>/dev/null || git checkout -q FETCH_HEAD
    else
        git checkout -q FETCH_HEAD
    fi
}

fetch_shallow() {
    if [ -n "$REF" ]; then
        if git fetch -q --depth="$FETCH_DEPTH" origin "$REF"; then
            return 0
        fi
        echo "checkout.sh: fetch by ref failed (${REF}), trying SHA" >&2
    fi
    if [ -n "$SHA" ]; then
        git fetch -q --depth="$FETCH_DEPTH" origin "$SHA"
        return $?
    fi
    return 1
}

if [ "$FETCH_DEPTH" = "0" ]; then
    git clone -q "$ORIGIN" .
    checkout_target
else
    if [ -d .git ]; then
        git remote set-url origin "$ORIGIN" 2>/dev/null || {
            git remote remove origin 2>/dev/null || true
            git remote add origin "$ORIGIN"
        }
    else
        git init -q
        git remote add origin "$ORIGIN"
    fi
    fetch_shallow || exit 1
    checkout_target
fi

echo "Checked out ${REPO} at $(git rev-parse --short HEAD)"
