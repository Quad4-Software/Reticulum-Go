# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
CARGO ?= cargo

.PHONY: all test check smoke examples clean librns

all: test

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

test: $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RNS_LIB_DIR="$(ABS_BIN)" \
	$(CARGO) test --manifest-path Cargo.toml

check: test

smoke: test

examples: $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RNS_LIB_DIR="$(ABS_BIN)" \
	$(MAKE) -C examples/smoke
	$(MAKE) -C examples/page-fetch
	$(MAKE) -C examples/pageserver

clean:
	$(CARGO) clean --manifest-path Cargo.toml
	$(MAKE) -C examples/smoke clean
	$(MAKE) -C examples/page-fetch clean
	$(MAKE) -C examples/pageserver clean
