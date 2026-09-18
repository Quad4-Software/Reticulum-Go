#!/bin/sh
# Warn when pull request commits lack signatures.
# GPG, SSH and gitsign (x509) signatures are all detected via git %G? codes:
#   G/U/X/Y count as signed, E counts as signed but locally unverifiable
#   (typical for gitsign commits without a configured verifier), N is unsigned.
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
unverifiable=0
while IFS= read -r line; do
	[ -n "$line" ] || continue
	code="${line%% *}"
	sha="${line#* }"
	case "$code" in
	G | U | X | Y | E)
		if [ "$code" = "E" ]; then
			unverifiable=$((unverifiable + 1))
			echo "signed-unverifiable: $sha $(git log -1 --format='%s' "$sha")"
		fi
		;;
	*)
		echo "unsigned: $sha $(git log -1 --format='%s' "$sha")"
		unsigned=$((unsigned + 1))
		;;
	esac
done <<EOF
$(git log --no-merges --format='%G? %H' "${BASE_SHA}..${HEAD_SHA}")
EOF

if [ "$unverifiable" -gt 0 ]; then
	echo ""
	echo "verify-signed-commits: $unverifiable commit(s) carry a signature that could not be" >&2
	echo "verify-signed-commits: verified locally (gitsign x509 or missing trust config)" >&2
fi

if [ "$unsigned" -gt 0 ]; then
	echo ""
	echo "verify-signed-commits: $unsigned commit(s) without a signature" >&2
	echo "verify-signed-commits: sign with git commit -S or gitsign (see CONTRIBUTING.md)" >&2
	exit 1
fi

echo "verify-signed-commits: all commits signed"
