# SPDX-License-Identifier: LicenseRef-Reticulum

ROOT := ../..
BIN := $(ROOT)/bin
RRC_LIB := $(BIN)/librrc.so
LXMF_LIB := $(BIN)/liblxmf.so
ABS_BIN := $(abspath $(BIN))
ABS_JAVA := $(abspath .)
JAVAC ?= javac
JAVA ?= java
JNA_VER := 5.17.0
JNA_JAR := $(ABS_JAVA)/lib/jna.jar
JNA_URL := https://repo1.maven.org/maven2/net/java/dev/jna/jna/$(JNA_VER)/jna-$(JNA_VER).jar
JNA_MIRROR_URL := https://maven-central.storage-download.googleapis.com/maven2/net/java/dev/jna/jna/$(JNA_VER)/jna-$(JNA_VER).jar
CURL_RETRY := curl -fsSL --retry 5 --retry-delay 3 --retry-all-errors
OUT := $(ABS_JAVA)/out
CP := $(OUT):$(JNA_JAR)

SRC := $(shell find src -type f -name '*.java' | sort)
TEST_SRC := tests/SmokeTest.java
LXMF_TEST_SRC := tests/LxmfSmokeTest.java

.PHONY: all test check smoke examples clean librrc liblxmf classes jna

all: test

librrc: $(RRC_LIB)

liblxmf: $(LXMF_LIB)

$(RRC_LIB):
	cd $(ROOT) && task build-librrc

$(LXMF_LIB):
	cd $(ROOT) && task build-liblxmf

jna: $(JNA_JAR)

$(JNA_JAR):
	mkdir -p $(ABS_JAVA)/lib
	$(CURL_RETRY) -o "$(ABS_JAVA)/lib/jna-$(JNA_VER).jar" "$(JNA_URL)" || \
		$(CURL_RETRY) -o "$(ABS_JAVA)/lib/jna-$(JNA_VER).jar" "$(JNA_MIRROR_URL)"
	ln -sfn "jna-$(JNA_VER).jar" "$(JNA_JAR)"

classes: $(JNA_JAR)
	mkdir -p $(OUT)
	$(JAVAC) -encoding UTF-8 -d $(OUT) -cp $(JNA_JAR) $(SRC)

test: $(RRC_LIB) $(LXMF_LIB) classes
	$(JAVAC) -encoding UTF-8 -d $(OUT) -cp $(CP) $(TEST_SRC) $(LXMF_TEST_SRC)
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	RRC_LIB_PATH="$(ABS_BIN)/librrc.so" \
	RRC_ROOT="$(abspath $(ROOT))" \
	$(JAVA) -cp $(CP) -Djna.library.path="$(ABS_BIN)" io.quad4.rrc.SmokeTest
	LIBRARY_PATH="$(ABS_BIN):$${LIBRARY_PATH:-}" \
	LD_LIBRARY_PATH="$(ABS_BIN):$${LD_LIBRARY_PATH:-}" \
	LXMF_LIB_PATH="$(ABS_BIN)/liblxmf.so" \
	LXMF_ROOT="$(abspath $(ROOT))" \
	$(JAVA) -cp $(CP) -Djna.library.path="$(ABS_BIN)" io.quad4.lxmf.LxmfSmokeTest

check: test

smoke: test

examples: $(RRC_LIB) $(LXMF_LIB) classes
	$(MAKE) -C examples/smoke
	$(MAKE) -C examples/hub-client
	$(MAKE) -C examples/lxmf-roundtrip

.PHONY: examples/hub-client examples/lxmf-roundtrip
examples/hub-client: $(RRC_LIB) classes
	$(MAKE) -C examples/hub-client

examples/lxmf-roundtrip: $(LXMF_LIB) classes
	$(MAKE) -C examples/lxmf-roundtrip

clean:
	rm -rf $(OUT)
