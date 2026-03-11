# Reticulum-Go

[![Revive Lint](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/revive.yml/badge.svg?branch=main)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/revive.yml)
[![Go Build](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/build.yml/badge.svg?branch=main)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/build.yml)
[![Go Test](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/go-test.yml/badge.svg?branch=main)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/go-test.yml)
[![Security Scans](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/scan.yml/badge.svg?branch=main)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/scan.yml)
[![Bearer](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/bearer.yml/badge.svg?branch=main)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/bearer.yml)

A high-performance Go implementation of the [Reticulum Network Stack](https://github.com/markqvist/Reticulum).

## Overview

Reticulum-Go provides full protocol compatibility with the Python reference implementation while leveraging Go's concurrency model for improved throughput and latency. The implementation targets cross-platform deployment across legacy and modern systems.

**Goals:**
- Full protocol interoperability with the Python reference implementation
- Cross-platform support for multiple architectures (old and new)
- High performance via Go's concurrency model
- Better privacy and security features that do not break compatibility with the Python reference implementation

## Requirements

- Go 1.25 or later

## Quick Start

### Build

```bash
make build
```

Output: `bin/reticulum-go`

### Install

Install to system path (default `/usr/local/bin`):

```bash
make install
```

Custom install prefix:

```bash
make install PREFIX=/opt/reticulum
```

### Run

```bash
make run
```

### Test

```bash
make test
```

## Makefile Reference

| Target | Description |
|--------|-------------|
| `make` / `make all` | Build release binary |
| `make build` | Build release binary (stripped, static) |
| `make install` | Build and install to PREFIX/bin |
| `make uninstall` | Remove installed binary |
| `make clean` | Remove build artifacts |
| `make test` | Run all tests |
| `make test-short` | Run short tests only |
| `make test-race` | Run tests with race detector |
| `make coverage` | Generate coverage report |
| `make bench` | Run benchmarks |
| `make fmt` | Format code |
| `make vet` | Run go vet |
| `make lint` | Run revive linter |
| `make check` | Run fmt, vet, lint, test-short |
| `make deps` | Download and verify dependencies |
| `make run` | Run with go run |
| `make debug` | Build debug binary |
| `make build-linux` | Cross-build for Linux (amd64, arm64, arm, riscv64) |
| `make build-windows` | Cross-build for Windows |
| `make build-darwin` | Cross-build for macOS |
| `make build-all` | Cross-build for Linux, Windows, macOS |

## Taskfile (Alternative)

The project also provides a [Taskfile](https://taskfile.dev/) for extended automation. Install Task and run `task --list` for available targets.

```bash
task build
task install
task test
```

Note: On some systems, use `go-task` instead of `task`; add `alias task='go-task'` to your shell config if needed.

## Development

### Nix

With Nix installed, use the development shell for a preconfigured environment:

```bash
nix develop
```

### Code Quality

```bash
make fmt
make vet
make lint
make check
```

### Cross-Platform Builds

```bash
make build-linux
make build-windows
make build-darwin
make build-all
```

## WebAssembly and Embedded

Build WebAssembly binary (requires Task):

```bash
task build-wasm
task test-wasm
```

For embedded systems and TinyGo builds, see the [tinygo branch](https://git.quad4.io/Networks/Reticulum-Go/src/branch/tinygo/). Requires TinyGo 0.37.0+.

## Experimental Features

### Green Tea GC

Build with experimental Green Tea garbage collector (Go 1.25+):

```bash
task build-experimental
```

## License

0BSD. See [LICENSE](LICENSE).
