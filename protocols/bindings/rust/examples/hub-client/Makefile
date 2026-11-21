# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../../../..
BIN := $(ROOT)/bin
RRC_LIB := $(BIN)/librrc.so
ABS_BIN := $(abspath $(BIN))
CARGO ?= cargo

.PHONY: all clean librrc run

all: run

librrc: $(RRC_LIB)

$(RRC_LIB):
	cd $(ROOT) && task build-librrc

run: librrc
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RRC_LIB_DIR="$(ABS_BIN)" \
	$(CARGO) run --manifest-path ../../Cargo.toml --example hub-client

clean:
