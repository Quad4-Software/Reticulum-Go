#!/bin/sh
# Docs tooling entry point. Usage: sh scripts/docs.sh <command>
#
#   setup    create .docs-venv and install the pinned docs requirements
#   bundle   build the ZIP/PDF/EPUB downloads into docs/en/assets/downloads
#   build    bundle + zensical build (output in site/)
#   serve    bundle + zensical serve on http://127.0.0.1:1313/
#   deploy   mike deploy <version> [latest] (local gh-pages, add --push to publish)
#
# Creates .docs-venv on first use with uv when available, else
# python3 -m venv. CI installs docs/requirements-docs.txt directly.
set -e
cd "$(dirname "$0")/.."

VENV=.docs-venv

setup() {
    if [ -x "$VENV/bin/zensical" ]; then
        return
    fi
    rm -rf "$VENV"
    if command -v uv >/dev/null 2>&1; then
        uv venv "$VENV" --seed
    else
        python3 -m venv "$VENV"
    fi
    "$VENV/bin/pip" install --require-hashes -r docs/requirements-docs.txt
}

cmd="${1:-help}"
case "$cmd" in
    setup)
        setup
        ;;
    bundle)
        setup
        exec "$VENV/bin/python" scripts/docs-bundle.py
        ;;
    build)
        setup
        "$VENV/bin/python" scripts/docs-bundle.py
        exec "$VENV/bin/zensical" build --strict
        ;;
    serve)
        setup
        "$VENV/bin/python" scripts/docs-bundle.py
        exec "$VENV/bin/zensical" serve
        ;;
    deploy)
        setup
        shift
        export PATH="$PWD/$VENV/bin:$PATH"
        exec "$VENV/bin/mike" deploy "$@"
        ;;
    *)
        sed -n '2,11p' "$0"
        exit 1
        ;;
esac
