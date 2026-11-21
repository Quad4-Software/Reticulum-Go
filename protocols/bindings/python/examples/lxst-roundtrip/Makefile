# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../../../..
BIN := $(ROOT)/bin
ABS_BIN := $(abspath $(BIN))
PYTHON ?= python3

.PHONY: all clean

all:
	PYTHONPATH=$(ROOT)/bindings/python \
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	LXST_LIB_PATH="$(ABS_BIN)/liblxst.so" \
	$(PYTHON) main.py

clean:
