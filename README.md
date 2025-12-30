# Reticulum-Go

[![Revive Lint](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/revive.yml/badge.svg?branch=main)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/revive.yml)
[![Go Build](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/build.yml/badge.svg?branch=main)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/build.yml)
[![Go Test](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/go-test.yml/badge.svg?branch=main)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/go-test.yml)
[![Gosec](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/gosec.yml/badge.svg?branch=main)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/gosec.yml)
[![Bearer](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/bearer.yml/badge.svg?branch=main)](https://git.quad4.io/Networks/Reticulum-Go/actions/workflows/bearer.yml)

A high-performance Go implementation of the [Reticulum Network Stack](https://github.com/markqvist/Reticulum)

## Project Goals:

- **Full Protocol Compatibility**: Maintain complete interoperability with the Python reference implementation
- **Cross-Platform Support**: Support for legacy and modern platforms across multiple architectures
- **Performance**: Leverage Go's concurrency model and runtime for improved throughput and latency
- **More Privacy and Security**: Additional privacy and security features beyond the base specification

## Prerequisites

- Go 1.24 or later
- [Task](https://taskfile.dev/) for build automation

Note: You may need to set `alias task='go-task'` in your shell configuration to use `task` instead of `go-task`.

### Nix

If you have Nix installed, you can use the development shell which automatically provides all dependencies including Task:

```bash
nix develop
```

This will enter a development environment with Go and Task pre-configured.

## Quick Start

### Building the Binary

```bash
task build
```

The compiled binary will be located in `bin/reticulum-go`.

### Running the Application

```bash
task run
```

### Running Tests

```bash
task test
```

## Development

### Code Quality

Format code:

```bash
task fmt
```

Run static analysis checks (formatting, vet, linting):

```bash
task check
```

### Testing

Run all tests:

```bash
task test
```

Run short tests only:

```bash
task test-short
```

Generate coverage report:

```bash
task coverage
```

### Benchmarking

Run benchmarks with standard GC:

```bash
task bench
```

Run benchmarks with experimental Green Tea GC:

```bash
task bench-experimental
```

Compare both GC implementations:

```bash
task bench-compare
```

## Tasks

The project uses [Task](https://taskfile.dev/) for all development and build operations.

```
| Task                | Description                                          |
|---------------------|------------------------------------------------------|
| default             | Show available tasks                                 |
| all                 | Clean, download dependencies, build and test         |
| build               | Build release binary (stripped, static)              |
| debug               | Build debug binary                                   |
| build-experimental  | Build with experimental Green Tea GC (Go 1.25+)      |
| experimental        | Alias for build-experimental                         |
| release             | Build stripped static binary for release             |
| fmt                 | Format Go code                                       |
| fmt-check           | Check if code is formatted (CI-friendly)             |
| vet                 | Run go vet                                           |
| lint                | Run revive linter                                    |
| scan                | Run gosec security scanner                           |
| check               | Run fmt-check, vet, and lint                         |
| bench               | Run benchmarks with standard GC                      |
| bench-experimental  | Run benchmarks with experimental GC                  |
| bench-compare       | Run benchmarks with both GC settings                 |
| clean               | Remove build artifacts                               |
| test                | Run all tests                                        |
| test-short          | Run short tests only                                 |
| test-race           | Run tests with race detector                         |
| coverage            | Generate test coverage report                        |
| checksum            | Generate SHA256 checksum for binary                  |
| deps                | Download and verify dependencies                     |
| mod-tidy            | Tidy go.mod file                                     |
| mod-verify          | Verify dependencies                                  |
| build-linux         | Build for Linux (amd64, arm64, arm, riscv64)         |
| build-all           | Build for all Linux architectures                    |
| build-wasm          | Build WebAssembly binary with standard Go compiler   |
| run                 | Run with go run                                      |
| tinygo-build        | Build binary with TinyGo compiler                    |
| tinygo-wasm         | Build WebAssembly binary with TinyGo                 |
| install             | Install dependencies                                 |

example: task build
```

## Cross-Platform Builds

### Linux Builds

Build for all Linux architectures:

```bash
task build-all
```

Build for specific Linux architecture:

```bash
task build-linux
```

## Embedded Systems and WebAssembly

For building for embedded systems, see the [tinygo branch](https://git.quad4.io/Networks/Reticulum-Go/src/branch/tinygo/). Requires TinyGo 0.37.0+.

Build WebAssembly binary with standard Go compiler:

```bash
task build-wasm
```

Build with TinyGo:

```bash
task tinygo-build
```

Build WebAssembly binary with TinyGo:

```bash
task tinygo-wasm
```

## Experimental Features

### Green Tea Garbage Collector

Build with experimental Green Tea GC (requires Go 1.25+):

```bash
task build-experimental
```

This enables the experimental garbage collector for performance evaluation and testing.

## License

This project is licensed under the [0BSD](LICENSE) license.