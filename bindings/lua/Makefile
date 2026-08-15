# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
ABS_LUA := $(abspath .)
LUAJIT ?= luajit
LUA_PATH := $(ABS_LUA)/?.lua;$(ABS_LUA)/?/init.lua;;

export LIBRARY_PATH := $(ABS_BIN):$(LIBRARY_PATH)
export LD_LIBRARY_PATH := $(ABS_BIN):$(LD_LIBRARY_PATH)
export RNS_LIB_PATH := $(ABS_BIN)/librns.so
export LUA_PATH

.PHONY: all test check smoke examples clean librns

all: test

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

test: $(RNS_LIB)
	$(LUAJIT) tests/smoke.lua

check: test

smoke: test

examples: $(RNS_LIB)
	$(MAKE) -C examples/smoke
	$(MAKE) -C examples/page-fetch
	$(MAKE) -C examples/pageserver

clean:
