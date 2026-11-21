# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../../../..
BIN := $(ROOT)/bin
LXMF_LIB := $(BIN)/liblxmf.so
ABS_BIN := $(abspath $(BIN))
KOTLIN_ROOT := $(abspath ../..)
JAVA_ROOT := $(abspath ../../../java)
JNA_JAR := $(JAVA_ROOT)/lib/jna.jar
JAVA_OUT := $(JAVA_ROOT)/out
OUT := $(KOTLIN_ROOT)/out
KOTLINC ?= kotlinc
JAVA ?= java
ifeq ($(KOTLIN_HOME),)
KOTLIN_HOME := $(abspath $(dir $(shell readlink -f "$$(command -v $(KOTLINC))" 2>/dev/null || command -v $(KOTLINC)))/..)
endif
KOTLIN_STDLIB := $(KOTLIN_HOME)/lib/kotlin-stdlib.jar
CP := $(OUT):$(JAVA_OUT):$(JNA_JAR):$(KOTLIN_STDLIB)

.PHONY: all clean libs run

all: run

libs:
	cd $(ROOT) && task build-liblxmf

run: libs
	$(MAKE) -C $(KOTLIN_ROOT) classes
	$(KOTLINC) -jvm-target 11 -cp $(CP) -d $(OUT) Main.kt
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	LXMF_LIB_PATH="$(ABS_BIN)/liblxmf.so" \
	LXMF_ROOT="$(abspath $(ROOT))" \
	$(JAVA) -cp $(CP) -Djna.library.path="$(ABS_BIN)" MainKt

clean:
