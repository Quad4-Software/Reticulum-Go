# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../../../..
BIN := $(ROOT)/bin
LXMF_LIB := $(BIN)/liblxmf.so
ABS_BIN := $(abspath $(BIN))
JAVA_ROOT := $(abspath ../..)
JNA_JAR := $(JAVA_ROOT)/lib/jna.jar
OUT := $(JAVA_ROOT)/out
CP := $(OUT):$(JNA_JAR)
JAVAC ?= javac
JAVA ?= java

.PHONY: all clean libs run

all: run

libs:
	cd $(ROOT) && task build-liblxmf

run: libs
	$(MAKE) -C $(JAVA_ROOT) classes
	$(JAVAC) -encoding UTF-8 -d $(OUT) -cp $(CP) Main.java
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	LXMF_LIB_PATH="$(ABS_BIN)/liblxmf.so" \
	LXMF_ROOT="$(abspath $(ROOT))" \
	$(JAVA) -cp $(CP) -Djna.library.path="$(ABS_BIN)" Main

clean:
