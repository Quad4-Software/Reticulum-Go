# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../../../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
JAVA_ROOT := $(abspath ../..)
JNA_JAR := $(JAVA_ROOT)/lib/jna.jar
OUT := $(JAVA_ROOT)/out
CP := $(OUT):$(JNA_JAR)
JAVAC ?= javac

.PHONY: all clean librns

all: check

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

check: $(RNS_LIB)
	$(MAKE) -C $(JAVA_ROOT) classes
	$(JAVAC) -encoding UTF-8 -d $(OUT) -cp $(CP) Main.java

clean:
