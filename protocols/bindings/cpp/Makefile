# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../..
BIN := $(ROOT)/bin
ABS_BIN := $(abspath $(BIN))
BUILD := build

.PHONY: all test examples clean libs

all: test

libs:
	cd $(ROOT) && task build-librrc && task build-libmf && task build-liblxmf

test: libs
	cmake -S . -B $(BUILD) -DRRC_LIB_DIR=$(ABS_BIN)
	cmake --build $(BUILD)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" ctest --test-dir $(BUILD) --output-on-failure

examples: test

clean:
	rm -rf $(BUILD)
