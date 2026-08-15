# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ODIN ?= odin
COLLECTION := -collection:rns=$(abspath ../..)
ABS_BIN := $(abspath $(BIN))
LINKER_FLAGS := -extra-linker-flags:"-L$(ABS_BIN) -Wl,-rpath,$(ABS_BIN)"

.PHONY: all clean librns

all: odin-smoke

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

odin-smoke: main.odin $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(ODIN) build . -out:$@ $(COLLECTION) $(LINKER_FLAGS)

clean:
	rm -f odin-smoke
