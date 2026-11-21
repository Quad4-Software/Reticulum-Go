# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../../../..
BIN := $(ROOT)/bin
ABS_BIN := $(abspath $(BIN))
CC ?= gcc
CFLAGS := -std=c11 -Wall -Wextra -I$(ROOT)/include
LDFLAGS := -L$(ABS_BIN) -llxmf -Wl,-rpath,$(ABS_BIN)

.PHONY: all clean

all:
	$(CC) $(CFLAGS) main.c -o lxmf-interop $(LDFLAGS)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" ./lxmf-interop

clean:
	rm -f lxmf-interop
