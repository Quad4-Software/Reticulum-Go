#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Cross-build librns for Linux host, Android ABIs, and Windows amd64.
# Usage:
#   sh scripts/build-librns-targets.sh [linux] [android] [windows] [all]
# Default: all targets that the local toolchain can support.

set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

GOCMD="${GOCMD:-go}"
BUILD_DIR="${BUILD_DIR:-bin}"
ANDROID_API="${ANDROID_API:-24}"
TARGETS="${*:-all}"

want() {
	case " $TARGETS " in
	*" all "*|*" $1 "*) return 0 ;;
	*) return 1 ;;
	esac
}

build_linux() {
	echo "==> linux amd64 (host) -> $BUILD_DIR/librns.so"
	mkdir -p "$BUILD_DIR"
	CGO_ENABLED=1 "$GOCMD" build -buildmode=c-shared -o "$BUILD_DIR/librns.so" ./cmd/librns
	cp include/rns.h "$BUILD_DIR/rns.h"
}

android_ndk_root() {
	if [ -n "${ANDROID_NDK_HOME:-}" ] && [ -d "$ANDROID_NDK_HOME" ]; then
		printf '%s\n' "$ANDROID_NDK_HOME"
		return 0
	fi
	if [ -n "${ANDROID_NDK_ROOT:-}" ] && [ -d "$ANDROID_NDK_ROOT" ]; then
		printf '%s\n' "$ANDROID_NDK_ROOT"
		return 0
	fi
	if [ -n "${ANDROID_HOME:-}" ] && [ -d "$ANDROID_HOME/ndk" ]; then
		# Prefer the highest installed NDK version.
		ls -1d "$ANDROID_HOME/ndk"/* 2>/dev/null | sort -V | tail -n1
		return 0
	fi
	if [ -d "$HOME/Android/Sdk/ndk" ]; then
		ls -1d "$HOME/Android/Sdk/ndk"/* 2>/dev/null | sort -V | tail -n1
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
	out="$BUILD_DIR/android/$abi/librns.so"
	echo "==> android $abi -> $out"
	mkdir -p "$(dirname "$out")"
	# shellcheck disable=SC2086
	env CGO_ENABLED=1 GOOS=android GOARCH="$goarch" $extra_env CC="$cc" \
		"$GOCMD" build -buildmode=c-shared -o "$out" ./cmd/librns
	cp include/rns.h "$BUILD_DIR/android/$abi/rns.h"
}

build_android() {
	ndk="$(android_ndk_root)" || {
		echo "skip android: set ANDROID_NDK_HOME or install Android NDK" >&2
		return 0
	}
	echo "using NDK: $ndk"
	build_android_abi arm64-v8a arm64 aarch64-linux-android "" "$ndk"
	build_android_abi armeabi-v7a arm armv7a-linux-androideabi "GOARM=7" "$ndk"
	build_android_abi x86_64 amd64 x86_64-linux-android "" "$ndk"
}

windows_cc() {
	if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
		printf '%s\n' "x86_64-w64-mingw32-gcc"
		return 0
	fi
	if command -v zig >/dev/null 2>&1; then
		printf '%s\n' "$ROOT/scripts/cc-windows-zig.sh"
		return 0
	fi
	return 1
}

build_windows() {
	cc="$(windows_cc)" || {
		echo "skip windows: need x86_64-w64-mingw32-gcc or zig" >&2
		return 0
	}
	out="$BUILD_DIR/windows/amd64/librns.dll"
	echo "==> windows amd64 (CC=$cc) -> $out"
	mkdir -p "$(dirname "$out")"
	# shellcheck disable=SC2086
	env CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC="$cc" \
		"$GOCMD" build -buildmode=c-shared -o "$out" ./cmd/librns
	cp include/rns.h "$BUILD_DIR/windows/amd64/rns.h"
}

if want linux; then
	build_linux
fi
if want android; then
	build_android
fi
if want windows; then
	build_windows
fi

echo "done"
