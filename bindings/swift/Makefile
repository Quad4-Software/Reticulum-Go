# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
ABS_INCLUDE := $(abspath $(ROOT)/include)
SWIFT ?= swift

SWIFT_FLAGS := \
	-Xcc -I$(ABS_INCLUDE) \
	-Xlinker -L$(ABS_BIN) \
	-Xlinker -lrns \
	-Xlinker -rpath \
	-Xlinker $(ABS_BIN)

.PHONY: all test check smoke examples clean librns

all: test

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

test: $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(SWIFT) test --package-path . $(SWIFT_FLAGS)

check: test

smoke: test

examples: $(RNS_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(SWIFT) run --package-path . RNSSmoke $(SWIFT_FLAGS)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(SWIFT) build --package-path . --product RNSPageFetch $(SWIFT_FLAGS)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	$(SWIFT) build --package-path . --product RNSPageserver $(SWIFT_FLAGS)

clean:
	rm -rf .build
