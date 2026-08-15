# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
KOTLIN_ROOT := $(abspath ../..)
JAVA_ROOT := $(abspath ../../../java)
JNA_JAR := $(JAVA_ROOT)/lib/jna.jar
JAVA_OUT := $(JAVA_ROOT)/out
OUT := $(KOTLIN_ROOT)/out
KOTLINC ?= kotlinc
ifeq ($(KOTLIN_HOME),)
KOTLIN_HOME := $(abspath $(dir $(shell readlink -f "$$(command -v $(KOTLINC))" 2>/dev/null || command -v $(KOTLINC)))/..)
endif
KOTLIN_STDLIB := $(KOTLIN_HOME)/lib/kotlin-stdlib.jar
CP := $(OUT):$(JAVA_OUT):$(JNA_JAR):$(KOTLIN_STDLIB)

.PHONY: all clean librns

all: check

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

check: $(RNS_LIB)
	$(MAKE) -C $(KOTLIN_ROOT) classes
	$(KOTLINC) -jvm-target 11 -cp $(CP) -d $(OUT) Main.kt

clean:
