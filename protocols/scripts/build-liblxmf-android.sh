#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum
# Cross-build liblxmf for Android ABIs (mirrors Reticulum-Go build-librns-targets android).
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

GOCMD="${GOCMD:-go}"
BUILD_DIR="${BUILD_DIR:-bin}"
ANDROID_API="${ANDROID_API:-24}"

android_ndk_root() {
	if [ -n "${ANDROID_NDK_HOME:-}" ] && [ -d "$ANDROID_NDK_HOME" ]; then
		printf '%s\n' "$ANDROID_NDK_HOME"
		return 0
	fi
	if [ -n "${ANDROID_NDK_ROOT:-}" ] && [ -d "$ANDROID_NDK_ROOT" ]; then
		printf '%s\n' "$ANDROID_NDK_ROOT"
		return 0
	fi
	if [ -d "$HOME/Android/Sdk/ndk" ]; then
		find "$HOME/Android/Sdk/ndk" -mindepth 1 -maxdepth 1 -type d -print 2>/dev/null | sort -V | tail -n1
		return 0
	fi
	return 1
}

build_android_abi() {
	abi="$1"
	goarch="$2"
	triple="$3"
	extra_env="$4"
	ndk="$5"
	prebuilt="$ndk/toolchains/llvm/prebuilt/linux-x86_64"
	cc="$prebuilt/bin/${triple}${ANDROID_API}-clang"
	if [ ! -x "$cc" ]; then
		echo "missing Android clang: $cc" >&2
		return 1
	fi
	out="$BUILD_DIR/android/$abi/liblxmf.so"
	echo "==> android $abi -> $out"
	mkdir -p "$(dirname "$out")"
	# shellcheck disable=SC2086
	env CGO_ENABLED=1 GOOS=android GOARCH="$goarch" $extra_env CC="$cc" \
		"$GOCMD" build -buildmode=c-shared -o "$out" ./cmd/liblxmf
	cp include/lxmf.h "$BUILD_DIR/android/$abi/lxmf.h"
}

ndk="$(android_ndk_root)" || {
	echo "set ANDROID_NDK_HOME or install Android NDK" >&2
	exit 1
}
echo "using NDK: $ndk"
build_android_abi arm64-v8a arm64 aarch64-linux-android "" "$ndk"
build_android_abi armeabi-v7a arm armv7a-linux-androideabi "GOARM=7" "$ndk"
build_android_abi x86_64 amd64 x86_64-linux-android "" "$ndk"
