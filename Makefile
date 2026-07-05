.PHONY: all build install uninstall clean test fmt vet lint vulncheck gosec check deps run
.PHONY: build-linux build-windows build-windows-legacy build-darwin build-all
.PHONY: test-short test-race test-crossref test-wasm test-all coverage bench debug release

GOCMD := go
GO_LEGACY_WIN7 ?= /usr/local/go-legacy-win7/bin/go
# Use committed vendor/ for builds and tests; targets that fetch modules or tools clear these.
GOFLAGS := -mod=vendor
GOPROXY := off
GOSUMDB := off
export GOFLAGS GOPROXY GOSUMDB
LIBS_ROOT ?= ../../Reticulum-Go-Projects
GOVULNCHECK_VER ?= v1.1.4
BINARY_NAME := reticulum-go
BUILD_DIR := bin
MAIN_PACKAGE := ./cmd/reticulum-go
PREFIX ?= /usr/local
INSTALL_DIR := $(PREFIX)/bin

all: build

build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GOCMD) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)

debug:
	@mkdir -p $(BUILD_DIR)
	$(GOCMD) build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)

release: build

install: build
	@mkdir -p $(INSTALL_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)"

uninstall:
	@rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Removed $(INSTALL_DIR)/$(BINARY_NAME)"

clean:
	$(GOCMD) clean
	rm -rf $(BUILD_DIR)

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
	$(GOCMD) test -run=^$ -bench=. -benchmem ./...

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

build-linux:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOCMD) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOCMD) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm $(GOCMD) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 $(GOCMD) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-riscv64 $(MAIN_PACKAGE)

build-windows:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOCMD) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GOCMD) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe $(MAIN_PACKAGE)

build-windows-legacy:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO_LEGACY_WIN7) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64-win7.exe $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO_LEGACY_WIN7) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64-win7.exe $(MAIN_PACKAGE)

build-darwin:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOCMD) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOCMD) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PACKAGE)

build-all: build-linux build-windows build-darwin
