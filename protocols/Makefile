# SPDX-License-Identifier: LicenseRef-Reticulum

.DEFAULT_GOAL := help

GO ?= go
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
GOARM ?= 6
VERSION ?= dev
OUTDIR ?= bin

PREFIX ?= /usr/local
DESTDIR ?=
BINDIR ?= $(PREFIX)/bin
LIBDIR ?= $(PREFIX)/lib
INCLUDEDIR ?= $(PREFIX)/include

export GOFLAGS ?= -mod=vendor
export GOPROXY ?= off
export GOSUMDB ?= off
export VERSION

GOARCH_TAG := $(GOARCH)
ifeq ($(GOARCH),arm)
GOARCH_TAG := armv$(GOARM)
endif

EXE :=
ifeq ($(GOOS),windows)
EXE := .exe
endif

ifeq ($(GOOS),linux)
LIB_EXT := so
else ifeq ($(GOOS),darwin)
LIB_EXT := dylib
else ifeq ($(GOOS),windows)
LIB_EXT := dll
else
LIB_EXT := so
endif

GORRCD_BIN := $(OUTDIR)/gorrcd-$(GOOS)-$(GOARCH_TAG)$(EXE)
GOLXMD_BIN := $(OUTDIR)/golxmd-$(GOOS)-$(GOARCH_TAG)$(EXE)
LIBRRC := $(OUTDIR)/librrc.$(LIB_EXT)
LIBMF := $(OUTDIR)/libmf.$(LIB_EXT)
LIBLXST := $(OUTDIR)/liblxst.$(LIB_EXT)

.PHONY: help all build daemons libs clean \
	gorrcd golxmd librrc libmf liblxmf liblxst \
	install install-daemons install-gorrcd install-golxmd install-libs \
	test test-short vet fmt check

help:
	@echo "Usage: make <target> [GOOS=...] [GOARCH=...] [PREFIX=...] [DESTDIR=...]"
	@echo ""
	@echo "Build:"
	@echo "  all, build, daemons   Build gorrcd and golxmd into $(OUTDIR)/"
	@echo "  gorrcd                Build gorrcd hub daemon"
	@echo "  golxmd                Build golxmd LXMF daemon"
	@echo "  libs                  Build librrc, libmf, liblxmf, and liblxst (CGO)"
	@echo "  librrc libmf liblxmf liblxst  Build one shared library"
	@echo ""
	@echo "Install:"
	@echo "  install               Install daemons to \$$(PREFIX)/bin (default $(PREFIX))"
	@echo "  install-daemons       Install gorrcd and golxmd"
	@echo "  install-gorrcd        Install gorrcd only"
	@echo "  install-golxmd        Install golxmd only"
	@echo "  install-libs          Install shared libs and headers"
	@echo ""
	@echo "Test and quality:"
	@echo "  test-short            Run short tests"
	@echo "  test                  Run all tests"
	@echo "  vet fmt check         Static checks"
	@echo ""
	@echo "Other:"
	@echo "  clean                 Remove $(OUTDIR)/ artifacts"
	@echo ""
	@echo "Examples:"
	@echo "  make golxmd"
	@echo "  make install-golxmd PREFIX=\$$HOME/.local"
	@echo "  make gorrcd GOOS=linux GOARCH=arm64"

all: build

build: daemons

daemons: gorrcd golxmd

libs: librrc libmf liblxmf liblxst

gorrcd:
	@sh scripts/ci/build-gorrcd.sh "$(GOOS)" "$(GOARCH)" "$(OUTDIR)"

golxmd:
	@sh scripts/ci/build-golxmd.sh "$(GOOS)" "$(GOARCH)" "$(OUTDIR)"

librrc:
	@sh scripts/ci/build-librrc.sh "$(GOOS)" "$(GOARCH)" "$(OUTDIR)"

libmf:
	@sh scripts/ci/build-libmf.sh "$(GOOS)" "$(GOARCH)" "$(OUTDIR)"

liblxmf:
	@sh scripts/ci/build-liblxmf.sh "$(GOOS)" "$(GOARCH)" "$(OUTDIR)"

liblxst:
	@sh scripts/ci/build-liblxst.sh "$(GOOS)" "$(GOARCH)" "$(OUTDIR)"

install: install-daemons

install-daemons: install-gorrcd install-golxmd

install-gorrcd: gorrcd
	@install -d "$(DESTDIR)$(BINDIR)"
	@install -m 755 "$(GORRCD_BIN)" "$(DESTDIR)$(BINDIR)/gorrcd$(EXE)"
	@echo "installed $(DESTDIR)$(BINDIR)/gorrcd$(EXE)"

install-golxmd: golxmd
	@install -d "$(DESTDIR)$(BINDIR)"
	@install -m 755 "$(GOLXMD_BIN)" "$(DESTDIR)$(BINDIR)/golxmd$(EXE)"
	@echo "installed $(DESTDIR)$(BINDIR)/golxmd$(EXE)"

install-libs: librrc libmf liblxmf
	@install -d "$(DESTDIR)$(LIBDIR)" "$(DESTDIR)$(INCLUDEDIR)"
	@install -m 755 "$(LIBRRC)" "$(DESTDIR)$(LIBDIR)/"
	@install -m 755 "$(LIBMF)" "$(DESTDIR)$(LIBDIR)/"
	@install -m 755 "$(LIBLXMF)" "$(DESTDIR)$(LIBDIR)/"
	@install -m 644 "$(OUTDIR)/rrc.h" "$(DESTDIR)$(INCLUDEDIR)/"
	@install -m 644 "$(OUTDIR)/mf.h" "$(DESTDIR)$(INCLUDEDIR)/"
	@install -m 644 "$(OUTDIR)/lxmf.h" "$(DESTDIR)$(INCLUDEDIR)/"
	@echo "installed libraries to $(DESTDIR)$(LIBDIR)"
	@echo "installed headers to $(DESTDIR)$(INCLUDEDIR)"

test-short:
	@$(GO) test -short -count=1 ./...

test:
	@$(GO) test -count=1 ./...

vet:
	@$(GO) vet ./...

fmt:
	@$(GO) fmt ./...

check: vet test-short

clean:
	@rm -rf "$(OUTDIR)/gorrcd-"* "$(OUTDIR)/golxmd-"* \
		"$(OUTDIR)/librrc."* "$(OUTDIR)/libmf."* "$(OUTDIR)/liblxmf."* \
		"$(OUTDIR)/rrc.h" "$(OUTDIR)/mf.h" "$(OUTDIR)/lxmf.h"
	@$(GO) clean
