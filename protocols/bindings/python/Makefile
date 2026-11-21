# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../..
BIN := $(ROOT)/bin
RRC_LIB := $(BIN)/librrc.so
MF_LIB := $(BIN)/libmf.so
LXMF_LIB := $(BIN)/liblxmf.so
LXST_LIB := $(BIN)/liblxst.so
ABS_BIN := $(abspath $(BIN))
PYTHON ?= python3

.PHONY: all test check smoke examples clean libs

all: test

libs: $(RRC_LIB) $(MF_LIB) $(LXMF_LIB) $(LXST_LIB)

$(RRC_LIB) $(MF_LIB) $(LXMF_LIB) $(LXST_LIB):
	cd $(ROOT) && task build-bindings

test: libs
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RRC_LIB_PATH="$(ABS_BIN)/librrc.so" \
	MF_LIB_PATH="$(ABS_BIN)/libmf.so" \
	LXMF_LIB_PATH="$(ABS_BIN)/liblxmf.so" \
	LXST_LIB_PATH="$(ABS_BIN)/liblxst.so" \
	PYTHONPATH=. \
	$(PYTHON) -m unittest discover -s tests -v

check: test

smoke: test

examples: libs
	$(MAKE) -C examples/smoke
	$(MAKE) -C examples/codec-roundtrip
	$(MAKE) -C examples/lxmf-roundtrip
	$(MAKE) -C examples/lxst-roundtrip
	$(MAKE) -C examples/hub-client

.PHONY: examples/hub-client examples/lxmf-roundtrip examples/lxst-roundtrip
examples/hub-client: libs
	$(MAKE) -C examples/hub-client

examples/lxmf-roundtrip: libs
	$(MAKE) -C examples/lxmf-roundtrip

examples/lxst-roundtrip: libs
	$(MAKE) -C examples/lxst-roundtrip

clean:
	rm -rf rrc/__pycache__ tests/__pycache__ *.egg-info
