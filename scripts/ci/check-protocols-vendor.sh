#!/bin/sh
# Verify the vendored copy of the parent module under protocols/vendor matches
# the repo root. protocols/go.mod replaces the parent module with the repo
# root, so a root change that protocols depends on must be re-vendored
# (cd protocols && go mod vendor) or protocols builds and tests run against
# stale copies. Works offline: it compares files, it does not run go mod vendor.
set -eu

WS="${GITEA_WORKSPACE:-${GITHUB_WORKSPACE:-.}}"
cd "$WS" || exit 1

vroot=protocols/vendor/git.quad4.io/reticulum/Reticulum-Go
if [ ! -d "$vroot" ]; then
	echo "check-protocols-vendor: $vroot missing" >&2
	exit 1
fi

stale=0
for f in $(cd "$vroot" && find . -type f | sed 's|^\./||' | sort); do
	if [ ! -f "$f" ]; then
		echo "check-protocols-vendor: vendored $f has no source in the repo root" >&2
		stale=1
	elif ! cmp -s "$f" "$vroot/$f"; then
		echo "check-protocols-vendor: $f differs from the vendored copy" >&2
		stale=1
	fi
done

if [ "$stale" -ne 0 ]; then
	echo "check-protocols-vendor: protocols/vendor is STALE. Fix with: (cd protocols && go mod vendor) and commit the result." >&2
	exit 1
fi
echo "check-protocols-vendor: protocols/vendor is in sync"
