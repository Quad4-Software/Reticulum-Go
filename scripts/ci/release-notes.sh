#!/bin/sh
# Emit goreleaser-style release notes markdown on stdout: commits since the
# previous tag grouped into sections by conventional commit type.
#
# Env:
#   RELEASE_TAG        tag being released (required)
#   GITHUB_REPOSITORY  owner/repo for commit links (required)
set -eu

TAG="${RELEASE_TAG:?RELEASE_TAG required}"
REPO="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY required}"
BASE_URL="https://github.com/${REPO}/commit"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Preview releases create the tag at publish time, so resolve the range end
# from RELEASE_TARGET/HEAD when the tag does not exist locally yet.
if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null 2>&1; then
	end="$TAG"
else
	end="${RELEASE_TARGET:-HEAD}"
fi

prev="$(git describe --tags --abbrev=0 "${end}^" 2>/dev/null || true)"
if [ -n "$prev" ]; then
	range="${prev}..${end}"
	heading="${prev}...${TAG}"
else
	range="$end"
	heading="$TAG"
fi

for b in feat fix perf docs test ci other; do
	: >"$TMP/$b"
done

# One line per commit: full SHA, tab, subject.
git log --no-merges --format='%H%x09%s' "$range" | while IFS="$(printf '\t')" read -r sha subj; do
	case "$subj" in
		feat:*|feat\(*\):*)        b=feat ;;
		fix:*|fix\(*\):*)          b=fix ;;
		perf:*|perf\(*\):*)        b=perf ;;
		docs:*|docs\(*\):*)        b=docs ;;
		test:*|tests:*|test\(*\):*|tests\(*\):*) b=test ;;
		ci:*|ci\(*\):*)            b=ci ;;
		*)                         b=other ;;
	esac
	printf '%s\t%s\n' "$sha" "$subj" >>"$TMP/$b"
done

emit() {
	file="$TMP/$1"
	title="$2"
	[ -s "$file" ] || return 0
	echo "### ${title}"
	echo
	while IFS="$(printf '\t')" read -r sha subj; do
		short="$(printf %s "$sha" | cut -c1-7)"
		echo "- ${subj} ([${short}](${BASE_URL}/${sha}))"
	done <"$file"
	echo
}

echo "## Changelog (${heading})"
echo
emit feat "New Features"
emit fix "Bug Fixes"
emit perf "Performance"
emit docs "Documentation"
emit test "Tests"
emit ci "CI/CD"
emit other "Other Changes"
