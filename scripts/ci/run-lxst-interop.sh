#!/bin/sh
# Run LXST Go/Python interop tests (requires pip lxst 0.5.4).
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

. "$ROOT/scripts/ci/env.sh"
export PATH="/usr/local/go/bin:${HOME:-/tmp}/.local/bin:${PATH}"

GOCMD="${GOCMD:-go}"
export REQUIRE_LXST="${REQUIRE_LXST:-1}"

if [ -z "${LXST_PYTHON:-}" ]; then
	if [ -x "$ROOT/.venv-lxst/bin/python" ]; then
		export LXST_PYTHON="$ROOT/.venv-lxst/bin/python"
	fi
fi

"$GOCMD" test -tags lxst_native -count=1 -timeout 25m -v \
	./pkg/lxst/proto/... \
	./pkg/lxst/phonebook/... \
	./pkg/lxst/call/... \
	./tests/lxst/integration/...
