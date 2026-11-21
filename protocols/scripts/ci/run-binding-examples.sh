#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum

set -eu

LANG="${1:-}"
if [ -z "$LANG" ]; then
	echo "usage: $0 <python|rust|java|lua>" >&2
	exit 1
fi

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

case "$LANG" in
python) sh scripts/ci/run-python-bindings.sh ;;
rust) sh scripts/ci/run-rust-bindings.sh ;;
java) sh scripts/ci/run-java-bindings.sh ;;
lua) sh scripts/ci/run-lua-bindings.sh ;;
*)
	echo "unknown language: $LANG" >&2
	exit 1
	;;
esac
