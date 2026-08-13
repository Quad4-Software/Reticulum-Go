#!/usr/bin/env sh
# Run in-repo gomutant mutation testing.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

WORKERS="${MUTATION_WORKERS:-4}"
TIMEOUT="${MUTATION_TIMEOUT:-45s}"
MAX="${MUTATION_MAX:-200}"

run_pkg() {
	pkg="$1"
	threshold="$2"
	max="$3"
	echo "mutation: $pkg (threshold=${threshold}%, max=${max})"
	go run ./tools/gomutant \
		-pkg "$pkg" \
		-threshold "$threshold" \
		-workers "$WORKERS" \
		-timeout "$TIMEOUT" \
		-max "$max"
}

failed=0
if [ -n "${MUTATION_PACKAGES:-}" ]; then
	threshold="${MUTATION_THRESHOLD:-55}"
	for pkg in $MUTATION_PACKAGES; do
		run_pkg "$pkg" "$threshold" "$MAX" || failed=1
	done
else
	run_pkg ./pkg/cryptography 55 "$MAX" || failed=1
	run_pkg ./pkg/packet 50 "$MAX" || failed=1
	run_pkg ./pkg/announce 40 "$MAX" || failed=1
	run_pkg ./pkg/destination 45 "$MAX" || failed=1
	run_pkg ./pkg/identity 50 "$MAX" || failed=1
	run_pkg ./pkg/ifac 45 "$MAX" || failed=1
	run_pkg ./pkg/backbone 40 "$MAX" || failed=1
	run_pkg ./pkg/interfaces 40 "$MAX" || failed=1
fi

if [ "$failed" -ne 0 ]; then
	exit 1
fi
echo "mutation: all packages passed"
