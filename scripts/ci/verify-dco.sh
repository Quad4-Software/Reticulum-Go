#!/bin/sh
# Require a Signed-off-by trailer on every pull request commit (DCO).
set -eu

if [ "${GITHUB_EVENT_NAME:-}" != "pull_request" ]; then
	exit 0
fi

BASE_SHA="${GITHUB_BASE_SHA:-}"
HEAD_SHA="${GITHUB_SHA:-HEAD}"
if [ -z "$BASE_SHA" ]; then
	echo "verify-dco: GITHUB_BASE_SHA not set, skipping"
	exit 0
fi

missing=0
while IFS= read -r sha; do
	[ -n "$sha" ] || continue
	if git log -1 --format='%(trailers)' "$sha" | grep -qE '^Signed-off-by: .+ <[^>]+>'; then
		continue
	fi
	echo "missing sign-off: $sha $(git log -1 --format='%s' "$sha")"
	missing=$((missing + 1))
done <<EOF
$(git rev-list --no-merges "${BASE_SHA}..${HEAD_SHA}")
EOF

if [ "$missing" -gt 0 ]; then
	echo ""
	echo "verify-dco: $missing commit(s) lack a Signed-off-by trailer" >&2
	echo "verify-dco: add with git commit -s (see CONTRIBUTING.md, DCO section)" >&2
	exit 1
fi

echo "verify-dco: all commits signed off"
