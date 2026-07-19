#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Build librns and run Java JNA bindings tests plus examples.

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if ! command -v javac >/dev/null 2>&1; then
	echo "javac not found on PATH" >&2
	exit 1
fi

if command -v task >/dev/null 2>&1; then
	task build-librns
else
	mkdir -p bin
	CGO_ENABLED=1 go build -buildmode=c-shared -o bin/librns.so ./cmd/librns
	cp include/rns.h bin/rns.h
fi

make -C bindings/java test
make -C bindings/java examples
