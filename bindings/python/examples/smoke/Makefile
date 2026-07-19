# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
PYTHON ?= python3

.PHONY: all clean librns

all: run

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

run: $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RNS_LIB_PATH="$(ABS_BIN)/librns.so" \
	PYTHONPATH="$(abspath ../..)" \
	$(PYTHON) main.py

clean:
