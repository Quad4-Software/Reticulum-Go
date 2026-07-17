# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../..
RNS := $(ROOT)/bin/reticulum-go

.PHONY: all build run clean example-pageserver

all: build

build: $(RNS)

$(RNS):
	cd $(ROOT) && task build

# Default pageserver for local demos. cwd is this directory so pages/ and files/ resolve.
# Extra flags: make run ARGS='-c /path/to/config'
run: build
	$(RNS) pageserver $(ARGS)

example-pageserver:
	go build -mod=mod -o $@ .

clean:
	rm -f example-pageserver
