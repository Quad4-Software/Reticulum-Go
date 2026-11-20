#!/bin/sh
# Hermeticity guard: CI must never touch the live public network.
#
# Fails when:
#   1. A workflow file enables a public-network opt-in variable
#      (RUN_VEGA_LIVE, RUN_PUBLIC_MESH).
#   2. A public host or IP appears in code that is not an allowlisted
#      opt-in test file.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

fail=0

# 1. Workflows must not enable public-network opt-ins.
for wf in .github/workflows/*.yml .github/workflows/*.yaml .forgejo/workflows/*.yml .forgejo/workflows/*.yaml; do
	[ -e "$wf" ] || continue
	if grep -nE 'RUN_VEGA_LIVE|RUN_PUBLIC_MESH|RUN_PUBLIC_NET' "$wf" >/dev/null 2>&1; then
		echo "check-hermetic: $wf sets a public-network opt-in variable:" >&2
		grep -nE 'RUN_VEGA_LIVE|RUN_PUBLIC_MESH|RUN_PUBLIC_NET' "$wf" >&2
		fail=1
	fi
done

# 2. Public hosts may only appear in allowlisted opt-in files.
PUBLIC='78\.46\.92\.205|rns\.beleth\.net|rns\.michmesh\.net|noderage|reticulum\.qortal|rns\.stoppedcold\.com|reticulum\.au|rns\.recipes|reticulum\.network'

# Files allowed to name public hosts: env-gated opt-in tests whose public
# defaults are unreachable without RUN_PUBLIC_MESH/RUN_VEGA_LIVE, files with
# URL mentions in comments or docs only, and this script itself.
ALLOWED='
scripts/ci/check-hermetic.sh
tests/interop/vega_backbone_live_test.go
tests/interop/nomadnet_relay_live_test.go
tests/interop/directory_outgoing_live_test.go
tests/interop/bz2_resource_live_test.go
tests/interop/harness/preflight.go
pkg/interfaces/i2p_directory_live_test.go
pkg/controlapi/doc.go
pkg/zenfix/rules.go
pkg/zenfix/accuracy_test.go
examples/page-downloader/main.go
'

is_allowed() {
	case "
$ALLOWED
" in
	*"
$1
"*) return 0 ;;
	esac
	return 1
}

hits=$(
	grep -rlnE "$PUBLIC" \
		--include='*.go' --include='*.py' --include='*.sh' \
		--include='*.yml' --include='*.yaml' \
		tests pkg cmd scripts examples 2>/dev/null |
		grep -v vendor/ | grep -v reticulum-ref/ || true
)

for f in $hits; do
	if ! is_allowed "$f"; then
		echo "check-hermetic: public host referenced in non-opt-in file: $f" >&2
		grep -nE "$PUBLIC" "$f" | head -5 >&2
		fail=1
	fi
done

if [ "$fail" -ne 0 ]; then
	echo "check-hermetic: FAILED (public-network access in CI)" >&2
	exit 1
fi
echo "check-hermetic: OK"
