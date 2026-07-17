# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ODIN ?= odin
COLLECTION := -collection:rns=$(ROOT)/bindings/odin
ABS_BIN := $(abspath $(BIN))
LINKER_FLAGS := -extra-linker-flags:"-L$(ABS_BIN) -Wl,-rpath,$(ABS_BIN)"
CONFIG ?= config.example

.PHONY: all clean librns run

all: odin-pageserver

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

odin-pageserver: main.odin $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(ODIN) build . -out:$@ $(COLLECTION) $(LINKER_FLAGS)

run: odin-pageserver
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	./odin-pageserver -c $(CONFIG) $(ARGS)

clean:
	rm -f odin-pageserver
