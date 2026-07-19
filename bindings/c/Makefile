# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so

.PHONY: all test examples clean librns

all: examples

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

examples: $(RNS_LIB)
	$(MAKE) -C examples/smoke
	$(MAKE) -C examples/page-fetch
	$(MAKE) -C examples/pageserver
	LIBRARY_PATH="$(ROOT)/bin:$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ROOT)/bin:$${LD_LIBRARY_PATH:-}" \
	./examples/smoke/librns-smoke

test: examples

clean:
	$(MAKE) -C examples/smoke clean
	$(MAKE) -C examples/page-fetch clean
	$(MAKE) -C examples/pageserver clean
