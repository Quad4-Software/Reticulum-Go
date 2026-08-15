# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
CARGO ?= cargo

.PHONY: all clean librns

all: rns-page-fetch

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

rns-page-fetch: $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(CARGO) build --release --manifest-path Cargo.toml

clean:
	$(CARGO) clean --manifest-path Cargo.toml
