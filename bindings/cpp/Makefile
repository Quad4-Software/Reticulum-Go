# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
BUILD_DIR := build
ABS_BIN := $(abspath $(BIN))

.PHONY: all test check smoke clean librns configure

all: test

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

configure: $(RNS_LIB)
	cmake -S . -B $(BUILD_DIR) \
		-DCMAKE_CXX_STANDARD=17 \
		-DRNS_LIB_DIR=$(ABS_BIN) \
		-DRNS_INCLUDE_DIR=$(abspath $(ROOT)/include)

test: configure
	cmake --build $(BUILD_DIR)
	cd $(BUILD_DIR) && \
		LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
		LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
		ctest --output-on-failure

check: test

smoke: test

clean:
	rm -rf $(BUILD_DIR)
