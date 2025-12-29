# Reticulum-Go

A Go implementation of the [Reticulum Network Stack](https://github.com/markqvist/Reticulum).

## Goals

- To be fully compatible with the original Python implementation.
- Support for a broader range of platforms and architectures legacy and modern.
- Additional privacy and security features.

## Development

### Prerequisites

- Go 1.24 or later
- [go-task](https://taskfile.dev/)

Might need `alias task='go-task'` in your shell to use it as `task` instead of `go-task`.

### Build

```bash
task build
```

### Run

```bash
task run
```

### Test

```bash
task test
```

### Format Code

```bash
task fmt
```

### Run All Checks

```bash
task check
```

## Embedded systems and WebAssembly

For building for WebAssembly and embedded systems, see the [tinygo branch](https://git.quad4.io/Networks/Reticulum-Go/src/branch/tinygo/). Requires TinyGo 0.37.0+. 

Note: I am not actively working on webassembly support at the moment.

```bash
task tinygo-build
task tinygo-wasm
```

### Experimental Features

Build with experimental Green Tea GC (Go 1.25+):

```bash
task build-experimental
```

## License

This project is licensed under the [0BSD](LICENSE) license.