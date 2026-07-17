# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

CC ?= gcc
ROOT := ../..
LIB := $(ROOT)/bin/librns.so
INC := $(ROOT)/include
CONFIG ?= config.example

.PHONY: all clean run librns

all: librns-pageserver

librns: $(LIB)

librns-pageserver: main.c $(LIB)
	$(CC) -I$(INC) -L$(ROOT)/bin -Wl,-rpath,'$$ORIGIN/../../bin' -o $@ main.c -lrns

$(LIB):
	$(MAKE) -C $(ROOT) build-librns 2>/dev/null || (cd $(ROOT) && task build-librns)

run: librns-pageserver
	./librns-pageserver -c $(CONFIG) $(ARGS)

clean:
	rm -f librns-pageserver
