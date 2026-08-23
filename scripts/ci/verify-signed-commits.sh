#!/bin/sh
# Warn when pull request commits lack GPG or SSH signatures.
set -eu

if [ "${GITHUB_EVENT_NAME:-}" != "pull_request" ]; then
	exit 0
fi

BASE_SHA="${GITHUB_BASE_SHA:-}"
HEAD_SHA="${GITHUB_SHA:-HEAD}"
if [ -z "$BASE_SHA" ]; then
	echo "verify-signed-commits: GITHUB_BASE_SHA not set, skipping"
	exit 0
fi

unsigned=0
while IFS= read -r sha; do
	[ -n "$sha" ] || continue
	if git verify-commit "$sha" >/dev/null 2>&1; then
		continue
	fi
	echo "unsigned: $sha $(git log -1 --format='%s' "$sha")"
	unsigned=$((unsigned + 1))
done <<EOF
$(git rev-list --no-merges "${BASE_SHA}..${HEAD_SHA}")
EOF

if [ "$unsigned" -gt 0 ]; then
	echo ""
	echo "verify-signed-commits: $unsigned commit(s) without verified signature" >&2
	echo "verify-signed-commits: sign with git commit -S when practical (see CONTRIBUTING.md)" >&2
	exit 1
fi

echo "verify-signed-commits: all commits signed"
