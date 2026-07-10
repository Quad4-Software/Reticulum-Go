# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Reticulum-Go build and install.
#
# Primary artifact is a single binary (reticulum-go) with subcommands:
#   daemon (default), status, id, probe, path, cp, pageserver
# Optional legacy tool names are installed as symlinks to that binary.

.PHONY: all build build-utils install uninstall clean test fmt vet lint vulncheck gosec check deps run help
.PHONY: build-linux build-windows build-windows-legacy build-darwin build-all
.PHONY: test-short test-race test-crossref test-wasm test-all coverage bench debug release
.PHONY: man install-man package package-deb package-rpm

.DEFAULT_GOAL := all

GOCMD := go
GO_LEGACY_WIN7 ?= /usr/local/go-legacy-win7/bin/go
# Use committed vendor/ for builds and tests; targets that fetch modules or tools clear these.
GOFLAGS := -mod=vendor
GOPROXY := off
GOSUMDB := off
export GOFLAGS GOPROXY GOSUMDB
LIBS_ROOT ?= ../../Reticulum-Go-Projects
GOVULNCHECK_VER ?= v1.1.4
NFPM_VER ?= v2.41.3

BINARY_NAME := reticulum-go
BUILD_DIR := bin
MAIN_PACKAGE := ./cmd/reticulum-go
PREFIX ?= /usr/local
DESTDIR ?=
BINDIR := $(PREFIX)/bin
MANDIR := $(PREFIX)/share/man
INSTALL_BINDIR := $(DESTDIR)$(BINDIR)
INSTALL_MANDIR := $(DESTDIR)$(MANDIR)

# Legacy CLI names installed as symlinks to $(BINARY_NAME).
TOOL_LINKS := rgostatus rgoid rgoprobe rgopath rgocp rgox rnx rgopageserver

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# Package version must start with a digit and avoid git describe noise.
PKG_VERSION ?= $(shell echo "$(VERSION)" | sed 's/^v//' | sed 's/-dirty$$//' | sed 's/-\([0-9]*\)-g[0-9a-f]*/.\1/' )
LDFLAGS := -s -w -X main.defaultVersion=$(VERSION)

all: build

help:
	@echo "Targets:"
	@echo "  build          Build $(BINARY_NAME) (daemon + tools)"
	@echo "  build-utils    Alias for build"
	@echo "  install        Install binary, tool symlinks, and man pages"
	@echo "  install-man    Install man pages only"
	@echo "  uninstall      Remove installed files"
	@echo "  package-deb    Build .deb into dist/ (nfpm)"
	@echo "  package-rpm    Build .rpm into dist/ (nfpm)"
	@echo "  test           Run tests"
	@echo "  check          fmt vet lint test-short vulncheck gosec"
	@echo "Variables: PREFIX=$(PREFIX) DESTDIR=$(DESTDIR) VERSION=$(VERSION)"

build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GOCMD) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)

# Compatibility alias: utilities live inside the main binary.
build-utils: build
	@echo "utilities are subcommands of $(BINARY_NAME) (status id probe path cp x pageserver)"

debug:
	@mkdir -p $(BUILD_DIR)
	$(GOCMD) build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)

release: build

install: build install-man
	@mkdir -p $(INSTALL_BINDIR)
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_BINDIR)/$(BINARY_NAME)
	@for name in $(TOOL_LINKS); do \
		ln -sfn $(BINARY_NAME) $(INSTALL_BINDIR)/$$name; \
	done
	@echo "Installed $(BINARY_NAME) and tool links to $(INSTALL_BINDIR)"
	@echo "Man pages installed under $(INSTALL_MANDIR)"

install-man:
	@mkdir -p $(INSTALL_MANDIR)/man1 $(INSTALL_MANDIR)/man8
	install -m 644 man/reticulum-go.1 $(INSTALL_MANDIR)/man1/reticulum-go.1
	install -m 644 man/reticulum-go-status.1 $(INSTALL_MANDIR)/man1/reticulum-go-status.1
	install -m 644 man/reticulum-go-id.1 $(INSTALL_MANDIR)/man1/reticulum-go-id.1
	install -m 644 man/reticulum-go-probe.1 $(INSTALL_MANDIR)/man1/reticulum-go-probe.1
	install -m 644 man/reticulum-go-path.1 $(INSTALL_MANDIR)/man1/reticulum-go-path.1
	install -m 644 man/reticulum-go-cp.1 $(INSTALL_MANDIR)/man1/reticulum-go-cp.1
	install -m 644 man/reticulum-go-x.1 $(INSTALL_MANDIR)/man1/reticulum-go-x.1
	install -m 644 man/reticulum-go-pageserver.1 $(INSTALL_MANDIR)/man1/reticulum-go-pageserver.1
	install -m 644 man/reticulum-go-debug.1 $(INSTALL_MANDIR)/man1/reticulum-go-debug.1
	install -m 644 man/reticulum-go.8 $(INSTALL_MANDIR)/man8/reticulum-go.8
	ln -sfn reticulum-go-status.1 $(INSTALL_MANDIR)/man1/rgostatus.1
	ln -sfn reticulum-go-id.1 $(INSTALL_MANDIR)/man1/rgoid.1
	ln -sfn reticulum-go-probe.1 $(INSTALL_MANDIR)/man1/rgoprobe.1
	ln -sfn reticulum-go-path.1 $(INSTALL_MANDIR)/man1/rgopath.1
	ln -sfn reticulum-go-cp.1 $(INSTALL_MANDIR)/man1/rgocp.1
	ln -sfn reticulum-go-x.1 $(INSTALL_MANDIR)/man1/rgox.1
	ln -sfn reticulum-go-x.1 $(INSTALL_MANDIR)/man1/rnx.1
	ln -sfn reticulum-go-pageserver.1 $(INSTALL_MANDIR)/man1/rgopageserver.1

uninstall:
	@rm -f $(INSTALL_BINDIR)/$(BINARY_NAME)
	@for name in $(TOOL_LINKS); do rm -f $(INSTALL_BINDIR)/$$name; done
	@rm -f $(INSTALL_MANDIR)/man1/reticulum-go.1 \
		$(INSTALL_MANDIR)/man1/reticulum-go-*.1 \
		$(INSTALL_MANDIR)/man1/rgo*.1 \
		$(INSTALL_MANDIR)/man1/rnx.1 \
		$(INSTALL_MANDIR)/man8/reticulum-go.8
	@echo "Removed $(BINARY_NAME), tool links, and man pages"

clean:
	$(GOCMD) clean
	rm -rf $(BUILD_DIR) dist

deps:
	sh scripts/vendor-sync.sh "$(LIBS_ROOT)"

test:
	$(GOCMD) run ./scripts/ci/testsummary -v ./...

test-short:
	$(GOCMD) run ./scripts/ci/testsummary -short -v ./...

test-race:
	CGO_ENABLED=1 $(GOCMD) run ./scripts/ci/testsummary -race -v ./...

test-crossref:
	@bash tests/crossref/run_crossref.sh

# js/wasm packages are not included in `go test ./...` on native GOOS; requires Node (see GOROOT/lib/wasm/go_js_wasm_exec).
test-wasm:
	env -i HOME=$$HOME PATH="/usr/local/bin:/usr/bin:/bin" GOROOT=$(shell go env GOROOT) TMPDIR=/tmp GOOS=js GOARCH=wasm $(GOCMD) test -count=1 -exec="$(shell go env GOROOT)/lib/wasm/go_js_wasm_exec" ./pkg/wasm/... ./cmd/reticulum-wasm/...

test-all: test test-wasm test-crossref

coverage:
	$(GOCMD) test -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

bench:
	$(GOCMD) test -run=^$$ -bench=. -benchmem ./...

fmt:
	$(GOCMD) fmt ./...

vet:
	$(GOCMD) vet ./...

lint:
	revive -config revive.toml -formatter friendly ./pkg/* ./cmd/* ./internal/*

vulncheck:
	env GOFLAGS= GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org,direct $(GOCMD) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VER) ./pkg/... ./cmd/... ./internal/... ./tests/...

# Scope packages so module cache under .cache/ is not scanned (avoids false positives from dependencies).
gosec:
	env GOFLAGS= GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=0 $(GOCMD) run github.com/securego/gosec/v2/cmd/gosec@latest -quiet -exclude-dir=vendor -exclude-dir=.cache ./pkg/... ./cmd/... ./internal/... ./tests/...

check: fmt vet lint test-short vulncheck gosec

run:
	$(GOCMD) run $(MAIN_PACKAGE)

man:
	@echo "Man pages live in man/ (installed by make install / make install-man)"
	@ls -1 man/*.1 man/*.8

package: package-deb package-rpm

package-deb: build
	@mkdir -p dist
	@ARCH=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/;s/armv7l/armhf/'); \
		VERSION="$(PKG_VERSION)" BINARY="$(CURDIR)/$(BUILD_DIR)/$(BINARY_NAME)" ARCH="$$ARCH" \
		env GOFLAGS= GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org,direct \
		$(GOCMD) run github.com/goreleaser/nfpm/v2/cmd/nfpm@$(NFPM_VER) package \
		--config packaging/nfpm.yaml --packager deb --target dist/

package-rpm: build
	@mkdir -p dist
	@ARCH=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/;s/armv7l/armhfp/'); \
		VERSION="$(PKG_VERSION)" BINARY="$(CURDIR)/$(BUILD_DIR)/$(BINARY_NAME)" ARCH="$$ARCH" \
		env GOFLAGS= GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org,direct \
		$(GOCMD) run github.com/goreleaser/nfpm/v2/cmd/nfpm@$(NFPM_VER) package \
		--config packaging/nfpm.yaml --packager rpm --target dist/

build-linux:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOCMD) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOCMD) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm $(GOCMD) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 $(GOCMD) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-riscv64 $(MAIN_PACKAGE)

build-windows:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOCMD) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GOCMD) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe $(MAIN_PACKAGE)

build-windows-legacy:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO_LEGACY_WIN7) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64-win7.exe $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO_LEGACY_WIN7) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64-win7.exe $(MAIN_PACKAGE)

build-darwin:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOCMD) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOCMD) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PACKAGE)

build-all: build-linux build-windows build-darwin
