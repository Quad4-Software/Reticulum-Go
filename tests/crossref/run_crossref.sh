#!/usr/bin/env bash
set -e

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
RETICULUM_REF="$ROOT_DIR/reticulum-ref"
REPO_URL="https://github.com/markqvist/Reticulum.git"

if [ -d "$RETICULUM_REF" ]; then
  echo "Updating reticulum-ref..."
  (cd "$RETICULUM_REF" && git fetch --depth 1 && git reset --hard origin/HEAD)
else
  echo "Cloning Python Reticulum..."
  git clone --depth 1 "$REPO_URL" "$RETICULUM_REF"
fi

export RETICULUM_PATH="$RETICULUM_REF"
cd "$(dirname "$0")"
python3 generate_vectors.py
cd "$ROOT_DIR"
go test -v ./tests/crossref/
