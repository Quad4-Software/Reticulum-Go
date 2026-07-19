# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ZIG ?= zig
ABS_BIN := $(abspath $(BIN))

.PHONY: all clean librns

all: zig-smoke

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

zig-smoke: main.zig build.zig $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(ZIG) build
	cp -f zig-out/bin/zig-smoke $@

clean:
	rm -rf zig-cache zig-out .zig-cache zig-smoke
