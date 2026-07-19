# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

DART ?= dart
ROOT := $(abspath ../..)

.PHONY: all test analyze get clean test-control test-ffi examples

all: test

get:
	$(DART) pub get

analyze: get
	$(DART) analyze --fatal-infos

test: analyze
	RNS_ROOT=$(ROOT) RNS_LIB_PATH=$(ROOT)/bin/librns.so $(DART) test

test-control: analyze
	$(DART) test test/models_test.dart test/client_mock_test.dart

test-ffi: analyze
	@test -f $(ROOT)/bin/librns.so || (echo "missing $(ROOT)/bin/librns.so (task build-librns)" >&2; exit 1)
	RNS_ROOT=$(ROOT) RNS_LIB_PATH=$(ROOT)/bin/librns.so $(DART) test test/ffi_smoke_test.dart

examples: get
	@test -f $(ROOT)/bin/librns.so || (echo "missing $(ROOT)/bin/librns.so (task build-librns)" >&2; exit 1)
	$(MAKE) -C examples/smoke

clean:
	rm -rf .dart_tool
	rm -f pubspec.lock
