# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../../../..
BIN := $(ROOT)/bin
ABS_BIN := $(abspath $(BIN))
ABS_JAVA := $(abspath ../..)
JNA_JAR := $(ABS_JAVA)/lib/jna.jar
OUT := $(ABS_JAVA)/out
CP := $(OUT):$(JNA_JAR)
JAVAC ?= javac
JAVA ?= java

.PHONY: all clean librrc run

all: run

librrc:
	cd $(ROOT) && task build-librrc

run: librrc
	$(MAKE) -C ../.. classes
	$(JAVAC) -encoding UTF-8 -d $(OUT) -cp $(CP) Main.java
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RRC_LIB_PATH="$(ABS_BIN)/librrc.so" \
	RRC_ROOT="$(abspath $(ROOT))" \
	$(JAVA) -cp $(CP) -Djna.library.path="$(ABS_BIN)" Main

clean:
