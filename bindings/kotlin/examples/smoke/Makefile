# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
KOTLIN_ROOT := $(abspath ../..)
JAVA_ROOT := $(abspath ../../../java)
JNA_JAR := $(JAVA_ROOT)/lib/jna.jar
JAVA_OUT := $(JAVA_ROOT)/out
OUT := $(KOTLIN_ROOT)/out
KOTLINC ?= kotlinc
JAVA ?= java
KOTLIN_HOME := $(abspath $(dir $(shell command -v $(KOTLINC)))/..)
KOTLIN_STDLIB := $(KOTLIN_HOME)/lib/kotlin-stdlib.jar
CP := $(OUT):$(JAVA_OUT):$(JNA_JAR):$(KOTLIN_STDLIB)

.PHONY: all clean librns

all: run

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

run: $(RNS_LIB)
	$(MAKE) -C $(KOTLIN_ROOT) classes
	$(KOTLINC) -jvm-target 11 -cp $(CP) -d $(OUT) Main.kt
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RNS_LIB_PATH="$(ABS_BIN)/librns.so" \
	RNS_ROOT="$(abspath $(ROOT))" \
	$(JAVA) -cp $(CP) -Djna.library.path="$(ABS_BIN)" MainKt

clean:
