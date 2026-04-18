# Reticulum-Go

[![Revive Lint](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/revive.yml/badge.svg?branch=master)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/revive.yml)
[![Go Build](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/build.yml/badge.svg?branch=master)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/build.yml)
[![Go Test](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/go-test.yml/badge.svg?branch=master)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/go-test.yml)
[![Security Scans](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/scan.yml/badge.svg?branch=master)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/scan.yml)

A high-performance and [secure](SECURITY.md) Go implementation of the [Reticulum Network Stack](https://github.com/markqvist/Reticulum).

## Overview

Reticulum-Go provides full protocol compatibility with the Python reference implementation while leveraging Go's concurrency model for improved throughput and latency. The implementation targets cross-platform deployment across legacy and modern systems.

See [COMPATIBILITY.md](COMPATIBILITY.md) for how this is verified against the Python stack and the [network API reference](https://reticulum.network/manual/reference.html).

**Goals:**
- Full protocol interoperability with the Python reference implementation
- Cross-platform support for multiple architectures (old and new)
- High performance via Go's concurrency model
- Improved privacy and security features that do not break compatibility with the Python reference implementation

### Cryptography

Cryptographic behaviour is centralized in `pkg/cryptography` (including a pluggable `CryptoProvider`). For deployments that need keys or signing outside process memory, Ed25519 signing can be delegated via `cryptography.Ed25519Signer` (for example a `crypto.Signer` backed by PKCS#11 or an HSM). The on-wire format stays fixed; replacing primitives or integrating hardware must remain coordinated with peers. Link encryption still uses the standard X25519/AES path unless you implement a compatible custom provider.

## Requirements

- Go 1.25.9 or later

## Quick Start

You can use the [Makefile](Makefile) targets below or run the equivalent `go` commands directly if you do not have Make installed.

### Build

```bash
make build
```

```bash
mkdir -p bin
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reticulum-go ./cmd/reticulum-go
```

Output: `bin/reticulum-go`

### Install

Install to system path (default `/usr/local/bin`):

```bash
make install
```

```bash
mkdir -p bin
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reticulum-go ./cmd/reticulum-go
cp bin/reticulum-go /usr/local/bin/
```

Custom install prefix:

```bash
make install PREFIX=/opt/reticulum
```

```bash
mkdir -p /opt/reticulum/bin
mkdir -p bin
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reticulum-go ./cmd/reticulum-go
cp bin/reticulum-go /opt/reticulum/bin/
```

Alternatively, install into your Go toolchain binary directory (`$GOBIN` or `$(go env GOPATH)/bin`):

```bash
CGO_ENABLED=0 go install -ldflags="-s -w" ./cmd/reticulum-go
```

### Run

```bash
make run
```

```bash
go run ./cmd/reticulum-go
```

### Test

```bash
make test
```

```bash
go test -v ./...
```

## Makefile Reference

| Target | Description | Go / other |
|--------|-------------|------------|
| `make` / `make all` | Build release binary | same as `make build` |
| `make build` | Build release binary (stripped, static) | `mkdir -p bin` then `CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reticulum-go ./cmd/reticulum-go` |
| `make install` | Build and install to PREFIX/bin | build as above, then `cp bin/reticulum-go $(PREFIX)/bin/` |
| `make uninstall` | Remove installed binary | `rm -f $(PREFIX)/bin/reticulum-go` |
| `make clean` | Remove build artifacts | `go clean` and `rm -rf bin` |
| `make test` | Run all tests | `go test -v ./...` |
| `make test-short` | Run short tests only | `go test -short -v ./...` |
| `make test-race` | Run tests with race detector | `go test -race -v ./...` |
| `make coverage` | Generate coverage report | `go test -coverprofile=coverage.out ./...` then `go tool cover -html=coverage.out` |
| `make bench` | Run benchmarks | `go test -run=^$ -bench=. -benchmem ./...` |
| `make fmt` | Format code | `go fmt ./...` |
| `make vet` | Run go vet | `go vet ./...` |
| `make lint` | Run revive linter | `revive -config revive.toml -formatter friendly ./pkg/* ./cmd/* ./internal/*` |
| `make vulncheck` | Run govulncheck | `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...` (override version with `GOVULNCHECK_VER`) |
| `make check` | Run fmt, vet, lint, test-short, vulncheck | run those targets in sequence |
| `make deps` | Download and verify dependencies | `go mod download` and `go mod verify` |
| `make run` | Run with go run | `go run ./cmd/reticulum-go` |
| `make debug` | Build debug binary | `mkdir -p bin` then `go build -o bin/reticulum-go ./cmd/reticulum-go` |
| `make build-linux` | Cross-build for Linux (amd64, arm64, arm, riscv64) | set `GOOS=linux` and `GOARCH=...` per [Makefile](Makefile) |
| `make build-windows` | Cross-build for Windows | set `GOOS=windows` and `GOARCH=...` per [Makefile](Makefile) |
| `make build-darwin` | Cross-build for macOS | set `GOOS=darwin` and `GOARCH=...` per [Makefile](Makefile) |
| `make build-all` | Cross-build for Linux, Windows, macOS | run the three cross-build command groups from the Makefile |

## Taskfile (Alternative)

The project also provides a [Taskfile](https://taskfile.dev/) for extended automation. Install Task and run `task --list` for available targets.

```bash
task build
task install
task test
```

Note: On some systems, use `go-task` instead of `task`; add `alias task='go-task'` to your shell config if needed.

## Development

### Code Quality

```bash
make fmt
make vet
make lint
make check
```

```bash
go fmt ./...
go vet ./...
revive -config revive.toml -formatter friendly ./pkg/* ./cmd/* ./internal/*
go test -short -v ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

### Cross-Platform Builds

```bash
make build-linux
make build-windows
make build-darwin
make build-all
```

Cross-compilation uses `GOOS` and `GOARCH` with the same `go build` flags as `make build`; see [Makefile](Makefile) `build-linux`, `build-windows`, and `build-darwin` targets for exact commands.

## WebAssembly and Embedded

Build WebAssembly binary (requires Task):

```bash
task build-wasm
task test-wasm
```

For embedded systems and TinyGo builds, see the [tinygo branch](https://git.quad4.io/Networks/Reticulum-Go/src/branch/tinygo/). Requires TinyGo 0.37.0+.

## License

0BSD. See [LICENSE](LICENSE).
