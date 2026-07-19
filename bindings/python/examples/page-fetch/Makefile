# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
PYTHON ?= python3

.PHONY: all clean librns

all: check

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

check: $(RNS_LIB)
	PYTHONPATH="$(abspath ../..)" $(PYTHON) -m py_compile main.py

clean:
	rm -rf __pycache__
