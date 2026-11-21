# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../../../..
BIN := $(ROOT)/bin
LXMF_LIB := $(BIN)/liblxmf.so
ABS_BIN := $(abspath $(BIN))
PYTHON ?= python3

.PHONY: all clean libs run

all: run

libs:
	cd $(ROOT) && task build-liblxmf

run: libs
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	LXMF_LIB_PATH="$(ABS_BIN)/liblxmf.so" \
	PYTHONPATH="$(abspath ../..)" \
	$(PYTHON) main.py

clean:
