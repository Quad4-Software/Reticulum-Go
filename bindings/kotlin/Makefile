# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
ABS_KOTLIN := $(abspath .)
JAVA_ROOT := $(abspath ../java)
JNA_JAR := $(JAVA_ROOT)/lib/jna.jar
JAVA_OUT := $(JAVA_ROOT)/out
OUT := $(ABS_KOTLIN)/out
KOTLINC ?= kotlinc
JAVA ?= java
KOTLIN_HOME := $(abspath $(dir $(shell command -v $(KOTLINC)))/..)
KOTLIN_STDLIB := $(KOTLIN_HOME)/lib/kotlin-stdlib.jar
CP := $(OUT):$(JAVA_OUT):$(JNA_JAR):$(KOTLIN_STDLIB)

.PHONY: all test check smoke examples clean librns classes

all: test

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

classes: $(RNS_LIB)
	$(MAKE) -C $(JAVA_ROOT) classes
	mkdir -p $(OUT)
	$(KOTLINC) -jvm-target 11 -cp $(JAVA_OUT):$(JNA_JAR):$(KOTLIN_STDLIB) -d $(OUT) src/io/quad4/rns/kotlin/RnsKt.kt

test: classes
	$(KOTLINC) -jvm-target 11 -cp $(CP) -d $(OUT) tests/SmokeTest.kt
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RNS_LIB_PATH="$(ABS_BIN)/librns.so" \
	RNS_ROOT="$(abspath $(ROOT))" \
	$(JAVA) -cp $(CP) -Djna.library.path="$(ABS_BIN)" io.quad4.rns.kotlin.SmokeTestKt

check: test

smoke: test

examples: classes
	$(MAKE) -C examples/smoke
	$(MAKE) -C examples/page-fetch
	$(MAKE) -C examples/pageserver

clean:
	rm -rf $(OUT)
