.PHONY: all build install uninstall clean test fmt vet lint check deps run
.PHONY: build-linux build-windows build-darwin build-all
.PHONY: test-short test-race test-crossref coverage bench debug release

GOCMD := go
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
	$(GOCMD) mod download
	$(GOCMD) mod verify

test:
	$(GOCMD) test -v ./...

test-short:
	$(GOCMD) test -short -v ./...

test-race:
	$(GOCMD) test -race -v ./...

test-crossref:
	@bash tests/crossref/run_crossref.sh

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

check: fmt vet lint test-short

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

build-darwin:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOCMD) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOCMD) build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PACKAGE)

build-all: build-linux build-windows build-darwin
