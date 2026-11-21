# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../../../..
BIN := $(ROOT)/bin
ABS_BIN := $(abspath $(BIN))
LUA ?= luajit

.PHONY: all clean librrc run

all: run

librrc:
	cd $(ROOT) && task build-librrc

run: librrc
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RRC_LIB_PATH="$(ABS_BIN)/librrc.so" \
	LUA_PATH="$(abspath ../..)/?/init.lua;$(abspath ../..)/rrc/?.lua;$(abspath ../..)/?.lua" \
	$(LUA) main.lua

clean:
