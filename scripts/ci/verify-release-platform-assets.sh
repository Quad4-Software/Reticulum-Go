#!/bin/sh
# Require linux-amd64 v1 and v3 together, plus linux-386 / linux-i686 aliases.
# Usage: verify-release-platform-assets.sh <directory>
set -eu

DIR="${1:?directory}"

need() {
	if [ ! -f "$DIR/$1" ]; then
		echo "verify-release-platform-assets: missing $DIR/$1" >&2
		exit 1
	fi
}

need reticulum-go-linux-amd64
need reticulum-go-linux-amd64-v1
need reticulum-go-linux-amd64-v3
need reticulum-go-linux-386
need reticulum-go-linux-i686

hash_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

h_base="$(hash_of "$DIR/reticulum-go-linux-amd64")"
h_v1="$(hash_of "$DIR/reticulum-go-linux-amd64-v1")"
h_v3="$(hash_of "$DIR/reticulum-go-linux-amd64-v3")"
h_386="$(hash_of "$DIR/reticulum-go-linux-386")"
h_i686="$(hash_of "$DIR/reticulum-go-linux-i686")"

if [ "$h_base" != "$h_v1" ]; then
	echo "verify-release-platform-assets: linux-amd64 must be GOAMD64=v1 (same bytes as linux-amd64-v1)" >&2
	exit 1
fi
if [ "$h_v1" = "$h_v3" ]; then
	echo "verify-release-platform-assets: linux-amd64-v3 must differ from v1" >&2
	exit 1
fi
if [ "$h_386" != "$h_i686" ]; then
	echo "verify-release-platform-assets: linux-i686 must match linux-386" >&2
	exit 1
fi

echo "verify-release-platform-assets: ok"
