# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../..
BIN := $(ROOT)/bin
ABS_BIN := $(abspath $(BIN))
CC ?= gcc
CFLAGS := -std=c11 -Wall -Wextra -I$(ROOT)/include
LDFLAGS := -L$(ABS_BIN) -lrrc -Wl,-rpath,$(ABS_BIN)

.PHONY: all test examples clean libs

all: test

libs:
	cd $(ROOT) && task build-librrc && task build-libmf && task build-liblxmf && task build-liblxst

test: examples

examples: libs
	$(MAKE) -C examples/smoke
	$(MAKE) -C examples/hub-client
	$(MAKE) -C examples/mf-smoke
	$(MAKE) -C examples/lxmf-smoke
	$(MAKE) -C examples/lxmf-interop
	$(MAKE) -C examples/lxst-smoke

.PHONY: examples/hub-client examples/lxmf-smoke examples/lxmf-interop examples/lxst-smoke
examples/hub-client: libs
	$(MAKE) -C examples/hub-client

examples/lxmf-smoke: libs
	$(MAKE) -C examples/lxmf-smoke

examples/lxmf-interop: libs
	$(MAKE) -C examples/lxmf-interop

examples/lxst-smoke: libs
	$(MAKE) -C examples/lxst-smoke

clean:
