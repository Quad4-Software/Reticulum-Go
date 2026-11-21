# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../../../..
BIN := $(ROOT)/bin
ABS_BIN := $(abspath $(BIN))
CC ?= gcc
CFLAGS := -std=c11 -Wall -Wextra -I$(ROOT)/include
LDFLAGS := -L$(ABS_BIN) -llxst -Wl,-rpath,$(ABS_BIN)

.PHONY: all clean

all:
	$(CC) $(CFLAGS) main.c -o lxst-smoke $(LDFLAGS)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" ./lxst-smoke

clean:
	rm -f lxst-smoke
