# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../..
BIN := $(ROOT)/bin
RRC_LIB := $(BIN)/librrc.so
ABS_BIN := $(abspath $(BIN))
CARGO ?= cargo

.PHONY: all test check smoke examples clean librrc

all: test

librrc: $(RRC_LIB)

$(RRC_LIB):
	cd $(ROOT) && task build-librrc

test: $(RRC_LIB)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RRC_LIB_DIR="$(ABS_BIN)" \
	$(CARGO) test --manifest-path Cargo.toml

check: test

smoke: test

examples: $(RRC_LIB)
	$(MAKE) -C examples/smoke
	$(MAKE) -C examples/hub-client

.PHONY: examples/hub-client
examples/hub-client: $(RRC_LIB)
	$(MAKE) -C examples/hub-client

clean:
	$(CARGO) clean --manifest-path Cargo.toml
