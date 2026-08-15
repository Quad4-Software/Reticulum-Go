# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_LUA := $(abspath ../..)
LUAJIT ?= luajit
LUA_PATH := $(ABS_LUA)/?.lua;$(ABS_LUA)/?/init.lua;;

.PHONY: all clean librns

all: check

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

check: $(RNS_LIB)
	LUA_PATH="$(LUA_PATH)" $(LUAJIT) -e 'assert(loadfile("main.lua"))'

clean:
