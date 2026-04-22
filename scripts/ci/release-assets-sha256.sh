#!/bin/sh
# Write a .sha256 sidecar next to each regular file in a directory.
# Usage: release-assets-sha256.sh <directory>
set -eu

DIR="${1:?directory}"
cd "$DIR" || exit 1
for file in *; do
    [ -f "$file" ] || continue
    case "$file" in
        *.sha256) continue ;;
    esac
    sha256sum "$file" | tee "${file}.sha256"
done
