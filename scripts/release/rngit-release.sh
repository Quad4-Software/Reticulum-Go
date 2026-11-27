#!/bin/sh
# Upload a reticulum-go release to the rngit remote repository.
#
# Creates rngit signatures (.rsg) and the release manifest (manifest.rsm)
# for every asset, checksum, SBOM and signature bundle of a tag, then
# uploads them with rngit release create. Release notes come from the
# matching CHANGELOG.md section.
#
# Usage:
#   sh scripts/release/rngit-release.sh <tag> [--dry-run] [--artifacts DIR]
#       [--remote rns://...] [--signer PATH]
#
# --dry-run (or -n) runs rngit release create -L, which generates the
# signatures and manifest locally without contacting the remote.
#
# Environment:
#   RNS_ID_PATH   signing identity (default ~/.rngit/client_identity)
#   RNGIT_REMOTE  remote repository URL (default below)
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

REMOTE="${RNGIT_REMOTE:-rns://06a54b505bb67b25ef3f8097e8001edc/public/Reticulum-Go}"
SIGNER="${RNS_ID_PATH:-$HOME/.rngit/client_identity}"
TAG=""
ARTIFACTS=""
DRY_RUN=0

while [ "$#" -gt 0 ]; do
	case "$1" in
	--dry-run|-n) DRY_RUN=1 ;;
	--artifacts) ARTIFACTS="${2:?}"; shift ;;
	--remote) REMOTE="${2:?}"; shift ;;
	--signer) SIGNER="${2:?}"; shift ;;
	-h|--help)
		sed -n '2,20p' "$0"
		exit 0 ;;
	-*)
		echo "rngit-release: unknown option $1" >&2; exit 2 ;;
	*)
		if [ -z "$TAG" ]; then TAG="$1"; else echo "rngit-release: unexpected argument $1" >&2; exit 2; fi ;;
	esac
	shift
done

[ -n "$TAG" ] || { echo "rngit-release: tag required" >&2; exit 2; }
git rev-parse --verify --quiet "refs/tags/$TAG" >/dev/null || {
	echo "rngit-release: tag $TAG not found locally; rngit requires the tag in this repository" >&2
	exit 1
}
[ -f "$SIGNER" ] || { echo "rngit-release: signing identity $SIGNER not found" >&2; exit 1; }
command -v rngit >/dev/null 2>&1 || { echo "rngit-release: rngit not installed" >&2; exit 1; }

WORK=""
cleanup() { [ -n "$WORK" ] && rm -rf "$WORK"; }
trap cleanup EXIT

# Resolve artifacts: explicit dir, else existing dist/, else gh download.
if [ -z "$ARTIFACTS" ]; then
	if [ -d "$ROOT/dist" ] && [ "$(find "$ROOT/dist" -type f | wc -l)" -gt 0 ]; then
		ARTIFACTS="$ROOT/dist"
	else
		command -v gh >/dev/null 2>&1 || {
			echo "rngit-release: no artifacts; pass --artifacts DIR or install gh to download $TAG assets" >&2
			exit 1
		}
		WORK="$(mktemp -d)"
		echo "Downloading $TAG assets with gh..."
		gh release download "$TAG" --dir "$WORK" || {
			echo "rngit-release: gh release download failed for $TAG" >&2
			exit 1
		}
		ARTIFACTS="$WORK"
	fi
fi
[ -d "$ARTIFACTS" ] || { echo "rngit-release: artifacts dir $ARTIFACTS missing" >&2; exit 1; }

# Extract the changelog section for this tag to use as release notes.
NOTES_FILE="$(mktemp "${TMPDIR:-/tmp}/rngit-notes.XXXXXX.md")"
trap 'rm -f "$NOTES_FILE"; [ -n "$WORK" ] && rm -rf "$WORK"' EXIT
awk -v tag="$TAG" '
	$0 ~ "^## +\\[?" tag "\\]?" { found=1; next }
	found && /^## / { exit }
	found { print }
' CHANGELOG.md | sed 's/^[[:space:]]*$//' | sed -e '1{/^$/d}' > "$NOTES_FILE"
if [ ! -s "$NOTES_FILE" ]; then
	printf '%s\n\nSee the release page for details.\n' "$TAG" > "$NOTES_FILE"
fi

# rngit asks for release notes through $EDITOR. Give it a helper that
# drops our changelog section into its scratch file.
EDITOR_HELPER="$(mktemp "${TMPDIR:-/tmp}/rngit-editor.XXXXXX.sh")"
printf '#!/bin/sh\ncat "%s" > "$1"\n' "$NOTES_FILE" > "$EDITOR_HELPER"
chmod +x "$EDITOR_HELPER"

TARGET="$TAG:$ARTIFACTS"
echo "Artifacts in $ARTIFACTS:"
find "$ARTIFACTS" -maxdepth 1 -type f -printf '  %f\n' | sort

if [ "$DRY_RUN" -eq 1 ]; then
	echo ""
	echo "Dry run: generating signatures and manifest locally with -L (no upload)."
	EDITOR="$EDITOR_HELPER" rngit release "$REMOTE" create "$TARGET" -s "$SIGNER" -L
	echo ""
	echo "Generated files:"
	find "$ARTIFACTS" -maxdepth 1 -type f \( -name '*.rsg' -o -name 'manifest.rsm' \) -printf '  %f\n' | sort
else
	echo "Uploading release $TAG to $REMOTE ..."
	EDITOR="$EDITOR_HELPER" rngit release "$REMOTE" create "$TARGET" -s "$SIGNER"
fi
rm -f "$EDITOR_HELPER"
