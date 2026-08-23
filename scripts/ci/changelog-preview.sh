#!/bin/sh
# Preview unreleased changelog section from conventional commits (git-cliff).
# Usage: changelog-preview.sh [from_ref]
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

FROM_REF="${1:-}"
if [ -z "$FROM_REF" ]; then
	if git describe --tags --abbrev=0 >/dev/null 2>&1; then
		FROM_REF="$(git describe --tags --abbrev=0)"
	else
		FROM_REF="HEAD~50"
	fi
fi

run_cliff() {
	if command -v git-cliff >/dev/null 2>&1; then
		git-cliff --config cliff.toml --unreleased --strip header "$FROM_REF..HEAD"
		return
	fi
	env GOFLAGS= GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org,direct \
		go run github.com/orhun/git-cliff@v2.7.0 \
		--config cliff.toml --unreleased --strip header "$FROM_REF..HEAD"
}

echo "changelog-preview: from $FROM_REF..HEAD"
echo "---"
run_cliff
