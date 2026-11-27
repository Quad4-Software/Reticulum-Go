#!/bin/sh
# Verify protocols/vendor is in sync with the parent module.
# protocols/go.mod replaces the parent module with the repo root, so any
# change to root pkg/ or internal/ code that protocols depends on must be
# re-vendored (cd protocols && go mod vendor) or protocols builds and tests
# run against stale copies.
#
# The check regenerates the vendor tree and compares it to the tree as it
# was before regeneration: if regeneration changes nothing, the vendor was
# in sync. Comparing against the pre-run state (not git HEAD) keeps the
# gate correct on dirty working trees as well as in CI.
set -eu

WS="${GITEA_WORKSPACE:-${GITHUB_WORKSPACE:-.}}"
cd "$WS" || exit 1

if [ ! -d protocols/vendor ]; then
	echo "check-protocols-vendor: protocols/vendor missing entirely" >&2
	exit 1
fi

snapshot() {
	find protocols/vendor -type f -print0 | sort -z | xargs -0 sha256sum 2>/dev/null | sha256sum | awk '{print $1}'
}

before=$(snapshot)

(
	cd protocols
	GOFLAGS='' go mod vendor
)

after=$(snapshot)

if [ "$before" = "$after" ]; then
	echo "check-protocols-vendor: protocols/vendor is in sync"
	exit 0
fi

echo "check-protocols-vendor: protocols/vendor is STALE." >&2
echo "A root-module change reached protocols/go.mod's replace target without" >&2
echo "re-vendoring. Fix with: (cd protocols && go mod vendor) and commit the result." >&2
git diff --stat -- protocols/vendor 2>/dev/null | tail -5 >&2 || true
exit 1
