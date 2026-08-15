# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
JAVA_ROOT := $(abspath ../..)
JNA_JAR := $(JAVA_ROOT)/lib/jna.jar
OUT := $(JAVA_ROOT)/out
CP := $(OUT):$(JNA_JAR)
JAVAC ?= javac
JAVA ?= java

.PHONY: all clean librns

all: run

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

run: $(RNS_LIB)
	$(MAKE) -C $(JAVA_ROOT) classes
	$(JAVAC) -encoding UTF-8 -d $(OUT) -cp $(CP) Main.java
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RNS_LIB_PATH="$(ABS_BIN)/librns.so" \
	RNS_ROOT="$(abspath $(ROOT))" \
	$(JAVA) -cp $(CP) -Djna.library.path="$(ABS_BIN)" Main

clean:
