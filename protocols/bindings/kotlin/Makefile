# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../..
BIN := $(ROOT)/bin
LXMF_LIB := $(BIN)/liblxmf.so
ABS_BIN := $(abspath $(BIN))
ABS_KOTLIN := $(abspath .)
JAVA_ROOT := $(abspath ../java)
JNA_JAR := $(JAVA_ROOT)/lib/jna.jar
JAVA_OUT := $(JAVA_ROOT)/out
OUT := $(ABS_KOTLIN)/out
KOTLINC ?= kotlinc
JAVA ?= java
ifeq ($(KOTLIN_HOME),)
KOTLIN_HOME := $(abspath $(dir $(shell readlink -f "$$(command -v $(KOTLINC))" 2>/dev/null || command -v $(KOTLINC)))/..)
endif
KOTLIN_STDLIB := $(KOTLIN_HOME)/lib/kotlin-stdlib.jar
CP := $(OUT):$(JAVA_OUT):$(JNA_JAR):$(KOTLIN_STDLIB)

.PHONY: all test check smoke examples clean liblxmf classes

all: test

liblxmf: $(LXMF_LIB)

$(LXMF_LIB):
	cd $(ROOT) && task build-liblxmf

classes: $(LXMF_LIB)
	$(MAKE) -C $(JAVA_ROOT) classes
	mkdir -p $(OUT)
	$(KOTLINC) -jvm-target 11 -cp $(JAVA_OUT):$(JNA_JAR):$(KOTLIN_STDLIB) -d $(OUT) src/io/quad4/lxmf/kotlin/LxmfKt.kt

test: classes
	$(KOTLINC) -jvm-target 11 -cp $(CP) -d $(OUT) tests/SmokeTest.kt
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	LXMF_LIB_PATH="$(ABS_BIN)/liblxmf.so" \
	LXMF_ROOT="$(abspath $(ROOT))" \
	$(JAVA) -cp $(CP) -Djna.library.path="$(ABS_BIN)" io.quad4.lxmf.kotlin.SmokeTestKt

check: test

smoke: test

examples: classes
	$(MAKE) -C examples/lxmf-roundtrip

.PHONY: examples/lxmf-roundtrip
examples/lxmf-roundtrip: classes
	$(MAKE) -C examples/lxmf-roundtrip

clean:
	rm -rf $(OUT)
