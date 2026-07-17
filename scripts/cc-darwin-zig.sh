#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Zig CC wrapper for cross-building librns for macOS.
# GOARCH selects the target (amd64 or arm64).

set -eu

arch="${GOARCH:-amd64}"
case "$arch" in
amd64) target="x86_64-macos" ;;
arm64) target="aarch64-macos" ;;
*)
	echo "unsupported GOARCH for darwin zig cc: $arch" >&2
	exit 1
	;;
esac

exec zig cc -target "$target" "$@"
