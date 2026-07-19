# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

PKG := ../..
ROOT := $(abspath ../../../..)
DART ?= dart

.PHONY: all clean run

all: run

run:
	cd $(PKG) && $(DART) pub get >/dev/null
	cd $(PKG) && RNS_ROOT=$(ROOT) RNS_LIB_PATH=$(ROOT)/bin/librns.so \
		$(DART) run --verbosity=error examples/smoke/main.dart

clean:
	@:
