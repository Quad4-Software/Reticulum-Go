#!/usr/bin/env bash
set -e



ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
RETICULUM_REF="$ROOT_DIR/reticulum-ref"
REPO_URL="rns://7649a50d84610232d1416b41d2896aff/reticulum/reticulum"

clone_or_update() {
  if [ -d "$RETICULUM_REF" ]; then
    echo "Updating reticulum-ref..."
    (cd "$RETICULUM_REF" && git fetch --depth 1 && git reset --hard origin/HEAD)
  else
    echo "Cloning Python Reticulum..."
    git clone --depth 1 "$REPO_URL" "$RETICULUM_REF"
  fi
}

generate_vectors() {
  clone_or_update
  export RETICULUM_PATH="$RETICULUM_REF"
  cd "$(dirname "$0")"
  python3 generate_vectors.py
  cd "$ROOT_DIR"
}

run_tests() {
  cd "$ROOT_DIR"
  go test -v ./tests/crossref/
}

case "${1:-all}" in
  generate)
    generate_vectors
    ;;
  test)
    run_tests
    ;;
  all)
    generate_vectors
    run_tests
    ;;
  *)
    echo "Usage: $0 {generate|test|all}"
    exit 1
    ;;
esac
