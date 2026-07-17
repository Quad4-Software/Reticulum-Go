# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ZIG ?= zig
ABS_BIN := $(abspath $(BIN))
CONFIG ?= config.example

.PHONY: all clean librns run

all: zig-pageserver

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

zig-pageserver: main.zig build.zig $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(ZIG) build
	cp -f zig-out/bin/zig-pageserver $@

run: zig-pageserver
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	./zig-pageserver -c $(CONFIG) $(ARGS)

clean:
	rm -rf zig-cache zig-out .zig-cache zig-pageserver
