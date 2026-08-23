# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Reticulum-Go build and install.
#
# Primary artifact is a single binary (reticulum-go) with subcommands:
#   daemon (default), status, id, probe, path, cp, pageserver
# Optional legacy tool names are installed as symlinks to that binary.

.PHONY: all build build-utils install uninstall clean test fmt vet lint staticcheck vulncheck gosec check prepush deps run help
.PHONY: build-linux build-windows build-windows-legacy build-windows-xp build-darwin build-all
.PHONY: build-freebsd build-openbsd build-netbsd build-dragonfly build-solaris build-illumos build-aix build-android
.PHONY: test-short test-race test-crossref test-wasm test-odin test-dart test-all coverage bench debug release
.PHONY: man install-man install-service package package-deb package-rpm package-arch stage-nfpm
.PHONY: test-services test-install-script tree-manifest tree-rsm-sign tree-rsm-verify hooks-install doctor bootstrap changelog-preview
.PHONY: build-librns
.PHONY: microvm-up microvm-stop microvm-kernel microvm-rootfs microvm-rebuild microvm-guest

.DEFAULT_GOAL := all

GOCMD := go
GO_LEGACY_WIN7 ?= /usr/local/go-legacy-win7/bin/go
GO_LEGACY_WINXP ?= /usr/local/go-legacy-winxp/bin/go
# Use committed vendor/ for builds and tests; targets that fetch modules or tools clear these.
GOFLAGS := -mod=vendor
GOPROXY := off
GOSUMDB := off
GOTOOLCHAIN ?= local
export GOFLAGS GOPROXY GOSUMDB GOTOOLCHAIN
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
# install-service: auto|systemd|openrc|runit|dinit|all
INIT ?= auto

# Legacy CLI names installed as symlinks to $(BINARY_NAME).
TOOL_LINKS := rgostatus rgoid rgoprobe rgopath rgocp rgox rnx rgosh rgopageserver rgogit git-remote-rns rgoslow rgoselfcheck rgospeed rgodump rgosnap rgozen

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
	@echo "  install-service Install init service files (INIT=$(INIT))"
	@echo "  test-install-script  shellcheck and dry-run for ./install.sh"
	@echo "  uninstall      Remove installed files"
	@echo "  package-deb    Build .deb into dist/ (nfpm)"
	@echo "  package-rpm    Build .rpm into dist/ (nfpm)"
	@echo "  package-arch   Build .pkg.tar.zst into dist/ (nfpm)"
	@echo "  test           Run tests"
	@echo "  test-odin      Build librns and run Odin bindings tests"
	@echo "  test-dart      Run Dart Control API client tests"
	@echo "  test-services  Docker tests for systemd/openrc/runit/dinit + logfile"
	@echo "  check          fmt-check vet lint staticcheck test-short vulncheck gosec"
	@echo "  tree-rsm-verify  Verify reticulum-go.rsm signature and hashes"
	@echo "  tree-rsm-sign    Sign tree inventory (requires RNS_ID_PATH)"
	@echo "  hooks-install    Enable .githooks (Go, YAML, shellcheck, commit-msg, pre-push)"
	@echo "  doctor           Verify dev tools match CI pins"
	@echo "  bootstrap        Install pinned task, revive, staticcheck"
	@echo "  prepush          fmt-check, vet, lint, test-short (pre-push hook)"
	@echo "  changelog-preview Preview unreleased CHANGELOG from commits"
	@echo "  microvm-up       Prepare and start Firecracker microvm + host bridge"
	@echo "  microvm-stop     Stop Firecracker microvm and host bridge"
	@echo "  microvm-kernel   Download Firecracker guest kernel"
	@echo "  microvm-rootfs   Build guest rootfs image"
	@echo "  microvm-rebuild  Force rebuild kernel/rootfs then up"
	@echo "  microvm-guest    Start microvm guest only"
	@echo "Variables: PREFIX=$(PREFIX) DESTDIR=$(DESTDIR) INIT=$(INIT) VERSION=$(VERSION)"

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
	install -m 644 man/reticulum-go-sh.1 $(INSTALL_MANDIR)/man1/reticulum-go-sh.1
	install -m 644 man/reticulum-go-pageserver.1 $(INSTALL_MANDIR)/man1/reticulum-go-pageserver.1
	install -m 644 man/reticulum-go-debug.1 $(INSTALL_MANDIR)/man1/reticulum-go-debug.1
	install -m 644 man/reticulum-go-slow.1 $(INSTALL_MANDIR)/man1/reticulum-go-slow.1
	install -m 644 man/reticulum-go-dump.1 $(INSTALL_MANDIR)/man1/reticulum-go-dump.1
	install -m 644 man/reticulum-go-snapshot.1 $(INSTALL_MANDIR)/man1/reticulum-go-snapshot.1
	install -m 644 man/reticulum-go-speedtest.1 $(INSTALL_MANDIR)/man1/reticulum-go-speedtest.1
	install -m 644 man/reticulum-go-self-check.1 $(INSTALL_MANDIR)/man1/reticulum-go-self-check.1
	install -m 644 man/reticulum-go.8 $(INSTALL_MANDIR)/man8/reticulum-go.8
	ln -sfn reticulum-go-status.1 $(INSTALL_MANDIR)/man1/rgostatus.1
	ln -sfn reticulum-go-id.1 $(INSTALL_MANDIR)/man1/rgoid.1
	ln -sfn reticulum-go-probe.1 $(INSTALL_MANDIR)/man1/rgoprobe.1
	ln -sfn reticulum-go-path.1 $(INSTALL_MANDIR)/man1/rgopath.1
	ln -sfn reticulum-go-cp.1 $(INSTALL_MANDIR)/man1/rgocp.1
	ln -sfn reticulum-go-x.1 $(INSTALL_MANDIR)/man1/rgox.1
	ln -sfn reticulum-go-x.1 $(INSTALL_MANDIR)/man1/rnx.1
	ln -sfn reticulum-go-sh.1 $(INSTALL_MANDIR)/man1/rgosh.1
	ln -sfn reticulum-go-pageserver.1 $(INSTALL_MANDIR)/man1/rgopageserver.1
	ln -sfn reticulum-go-slow.1 $(INSTALL_MANDIR)/man1/rgoslow.1
	ln -sfn reticulum-go-speedtest.1 $(INSTALL_MANDIR)/man1/rgospeed.1
	ln -sfn reticulum-go-dump.1 $(INSTALL_MANDIR)/man1/rgodump.1
	ln -sfn reticulum-go-snapshot.1 $(INSTALL_MANDIR)/man1/rgosnap.1
	ln -sfn reticulum-go-self-check.1 $(INSTALL_MANDIR)/man1/rgoselfcheck.1

install-service:
	sh scripts/install-service.sh --prefix "$(PREFIX)" --destdir "$(DESTDIR)" --bindir "$(BINDIR)" --init "$(INIT)"

test-install-script:
	sh scripts/ci/test-install.sh

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
	$(MAKE) -C bindings/odin clean
	$(MAKE) -C bindings/dart clean

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
	env -i HOME=$$HOME PATH="/usr/local/bin:/usr/bin:/bin" GOROOT=$(shell go env GOROOT) TMPDIR=/tmp TESTSUMMARY_GOOS=js TESTSUMMARY_GOARCH=wasm $(GOCMD) run ./scripts/ci/testsummary -count=1 -exec="$(shell go env GOROOT)/lib/wasm/go_js_wasm_exec" ./pkg/wasm/... ./cmd/reticulum-wasm/...

build-librns:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 $(GOCMD) build -buildmode=c-shared -o $(BUILD_DIR)/librns.so ./cmd/librns
	cp include/rns.h $(BUILD_DIR)/rns.h

test-odin: build-librns
	$(MAKE) -C bindings/odin test

test-dart:
	$(MAKE) -C bindings/dart test

test-all: test test-wasm test-crossref test-odin test-dart

test-services:
	sh scripts/ci/test-services-docker.sh

test-self-check:
	sh scripts/ci/run-self-check.sh

test-self-check-riscv64:
	sh scripts/ci/run-qemu-arch-self-check.sh riscv64

test-self-check-386:
	sh scripts/ci/run-qemu-arch-self-check.sh 386

test-self-check-arm:
	GOARM=6 sh scripts/ci/run-qemu-arch-self-check.sh arm

test-self-check-ppc64le:
	sh scripts/ci/run-qemu-arch-self-check.sh ppc64le

test-self-check-ppc64:
	sh scripts/ci/run-qemu-arch-self-check.sh ppc64

coverage:
	$(GOCMD) test -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

bench:
	$(GOCMD) test -run=^$$ -bench=. -benchmem ./...

fmt:
	$(GOCMD) fmt ./...

fmt-check:
	@UNFORMATTED=$$(gofmt -l $$(git ls-files '*.go' ':!:vendor/**' ':!:**/vendor/**')); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "Code is not formatted. Run 'make fmt' to fix."; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	fi

vet:
	$(GOCMD) vet ./...

lint:
	revive -config revive.toml -formatter friendly ./pkg/* ./cmd/* ./internal/*

staticcheck:
	staticcheck -tests=false ./pkg/... ./cmd/... ./internal/... ./tests/...

vulncheck:
	env GOFLAGS= GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org,direct $(GOCMD) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VER) ./pkg/... ./cmd/... ./internal/... ./tests/...

# Scope packages so module cache under .cache/ is not scanned (avoids false positives from dependencies).
gosec:
	env GOFLAGS= GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=0 $(GOCMD) run github.com/securego/gosec/v2/cmd/gosec@latest -quiet -exclude-dir=vendor -exclude-dir=.cache -exclude-dir=testdata ./pkg/... ./cmd/... ./internal/... ./tests/...

prepush: fmt-check vet lint test-short

check: fmt vet lint staticcheck test-short vulncheck gosec

doctor:
	sh scripts/ci/doctor.sh

bootstrap:
	sh scripts/ci/bootstrap.sh

changelog-preview:
	sh scripts/ci/changelog-preview.sh

run:
	$(GOCMD) run $(MAIN_PACKAGE)

man:
	@echo "Man pages live in man/ (installed by make install / make install-man)"
	@ls -1 man/*.1 man/*.8

package: package-deb package-rpm package-arch

stage-nfpm:
	sh scripts/stage-nfpm-units.sh

package-deb: build stage-nfpm
	@mkdir -p dist
	@ARCH=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/;s/armv7l/armhf/'); \
		VERSION="$(PKG_VERSION)" BINARY="$(CURDIR)/$(BUILD_DIR)/$(BINARY_NAME)" ARCH="$$ARCH" \
		env GOFLAGS= GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org,direct \
		$(GOCMD) run github.com/goreleaser/nfpm/v2/cmd/nfpm@$(NFPM_VER) package \
		--config packaging/nfpm.yaml --packager deb --target dist/

package-rpm: build stage-nfpm
	@mkdir -p dist
	@ARCH=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/;s/armv7l/armhfp/'); \
		VERSION="$(PKG_VERSION)" BINARY="$(CURDIR)/$(BUILD_DIR)/$(BINARY_NAME)" ARCH="$$ARCH" \
		env GOFLAGS= GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org,direct \
		$(GOCMD) run github.com/goreleaser/nfpm/v2/cmd/nfpm@$(NFPM_VER) package \
		--config packaging/nfpm.yaml --packager rpm --target dist/

package-arch: build stage-nfpm
	@mkdir -p dist
	@ARCH=$$(uname -m | sed 's/armv7l/armv7h/'); \
		VERSION="$(PKG_VERSION)" BINARY="$(CURDIR)/$(BUILD_DIR)/$(BINARY_NAME)" ARCH="$$ARCH" \
		env GOFLAGS= GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org,direct \
		$(GOCMD) run github.com/goreleaser/nfpm/v2/cmd/nfpm@$(NFPM_VER) package \
		--config packaging/nfpm.yaml --packager archlinux --target dist/

build-linux:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh linux

build-windows:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh windows

build-windows-legacy:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO_LEGACY_WIN7) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64-win7.exe $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO_LEGACY_WIN7) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64-win7.exe $(MAIN_PACKAGE)

build-windows-xp:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=386 $(GO_LEGACY_WINXP) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-386-winxp.exe $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO_LEGACY_WINXP) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64-winxp.exe $(MAIN_PACKAGE)

build-darwin:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh darwin

build-freebsd:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh freebsd

build-openbsd:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh openbsd

build-netbsd:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh netbsd

build-dragonfly:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh dragonfly

build-solaris:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh solaris

build-illumos:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh illumos

build-aix:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh aix

build-android:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh android

build-all:
	DEST_DIR=$(BUILD_DIR) VERSION="$(VERSION)" sh scripts/build-release-targets.sh

tree-manifest:
	sh scripts/ci/tree-manifest.sh generate

tree-rsm-verify:
	sh scripts/ci/verify-tree-rsm.sh

tree-rsm-sign:
	sh scripts/ci/sign-tree-rsm.sh

hooks-install:
	sh scripts/ci/install-git-hooks.sh

microvm-up:
	./microvm/up.sh

microvm-stop:
	./microvm/stop.sh

microvm-kernel:
	./microvm/fetch-kernel.sh

microvm-rootfs:
	./microvm/build-rootfs.sh

microvm-rebuild:
	./microvm/up.sh --rebuild

microvm-guest:
	./microvm/up.sh --guest-only
