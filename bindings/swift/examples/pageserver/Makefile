# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
ABS_INCLUDE := $(abspath $(ROOT)/include)
SWIFT ?= swift
PKG := $(abspath ../..)

SWIFT_FLAGS := \
	-Xcc -I$(ABS_INCLUDE) \
	-Xlinker -L$(ABS_BIN) \
	-Xlinker -lrns \
	-Xlinker -rpath \
	-Xlinker $(ABS_BIN)

.PHONY: all clean librns

all: check

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

check: $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(SWIFT) build --package-path "$(PKG)" --product RNSPageserver $(SWIFT_FLAGS)

clean:
