# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

ROOT := ../..
BIN := $(ROOT)/bin
RNS_LIB := $(BIN)/librns.so
ABS_BIN := $(abspath $(BIN))
ABS_JAVA := $(abspath .)
JAVAC ?= javac
JAVA ?= java
JNA_VER := 5.17.0
JNA_JAR := $(ABS_JAVA)/lib/jna.jar
JNA_URL := https://repo1.maven.org/maven2/net/java/dev/jna/jna/$(JNA_VER)/jna-$(JNA_VER).jar
OUT := $(ABS_JAVA)/out
CP := $(OUT):$(JNA_JAR)

SRC := $(shell find src -name '*.java' | sort)
TEST_SRC := tests/SmokeTest.java

.PHONY: all test check smoke examples clean librns classes jna

all: test

librns: $(RNS_LIB)

$(RNS_LIB):
	cd $(ROOT) && task build-librns

jna: $(JNA_JAR)

$(JNA_JAR):
	mkdir -p $(ABS_JAVA)/lib
	curl -fsSL -o "$(ABS_JAVA)/lib/jna-$(JNA_VER).jar" "$(JNA_URL)"
	ln -sfn "jna-$(JNA_VER).jar" "$(JNA_JAR)"

classes: $(JNA_JAR)
	mkdir -p $(OUT)
	$(JAVAC) -encoding UTF-8 -d $(OUT) -cp $(JNA_JAR) $(SRC)

test: $(RNS_LIB) classes
	$(JAVAC) -encoding UTF-8 -d $(OUT) -cp $(CP) $(TEST_SRC)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RNS_LIB_PATH="$(ABS_BIN)/librns.so" \
	RNS_ROOT="$(abspath $(ROOT))" \
	$(JAVA) -cp $(CP) -Djna.library.path="$(ABS_BIN)" io.quad4.rns.SmokeTest

check: test

smoke: test

examples: $(RNS_LIB) classes
	$(MAKE) -C examples/smoke
	$(MAKE) -C examples/page-fetch
	$(MAKE) -C examples/pageserver

clean:
	rm -rf $(OUT)
