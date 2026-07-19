# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
ABS_LUA := $(abspath ../..)
LUAJIT ?= luajit
LUA_PATH := $(ABS_LUA)/?.lua;$(ABS_LUA)/?/init.lua;;

.PHONY: all clean librns

all: run

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

run: $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RNS_LIB_PATH="$(ABS_BIN)/librns.so" \
	LUA_PATH="$(LUA_PATH)" \
	$(LUAJIT) main.lua

clean:
