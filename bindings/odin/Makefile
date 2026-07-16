# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ODIN ?= odin
COLLECTION := -collection:rns=$(CURDIR)
ABS_BIN := $(abspath $(BIN))
LINKER_FLAGS := -extra-linker-flags:"-L$(ABS_BIN) -Wl,-rpath,$(ABS_BIN)"
TEST_FLAGS := -define:ODIN_TEST_FANCY=false -define:ODIN_TEST_THREADS=1

.PHONY: all test check smoke clean librns

all: test

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

test: $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(ODIN) test tests $(COLLECTION) $(LINKER_FLAGS) $(TEST_FLAGS)

check: test

smoke: $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(ODIN) test tests $(COLLECTION) $(LINKER_FLAGS) $(TEST_FLAGS) \
		-define:ODIN_TEST_NAMES=test_version,test_node_lifecycle

clean:
	rm -f tests/rns_tests
