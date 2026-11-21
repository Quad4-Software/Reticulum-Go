# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../../../..
BIN := $(ROOT)/bin
RRC_LIB := $(BIN)/librrc.so
ABS_BIN := $(abspath $(BIN))
PYTHON ?= python3

.PHONY: all clean librrc

all: run

librrc: $(RRC_LIB)

$(RRC_LIB):
	cd $(ROOT) && task build-librrc

run: $(RRC_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RRC_LIB_PATH="$(ABS_BIN)/librrc.so" \
	PYTHONPATH="$(abspath ../..)" \
	$(PYTHON) main.py

clean:
