# Reticulum-Go

A high-performance and [secure](SECURITY.md) Go implementation of the [Reticulum Network Stack](https://github.com/markqvist/Reticulum).

## Overview

Reticulum-Go provides full protocol compatibility with the Python reference implementation while leveraging Go's concurrency model for improved throughput and latency. The implementation targets cross-platform deployment across legacy and modern systems.

See [COMPATIBILITY.md](COMPATIBILITY.md) for how this is verified against the Python stack and the [network API reference](https://reticulum.network/manual/reference.html).

## Features

| Area | Status | Notes |
|------|:------:|-------|
| Wire compatibility with Python [Reticulum](https://github.com/markqvist/Reticulum) | Yes | Packet and crypto paths cross-checked (`tests/crossref`, interop tests); see [COMPATIBILITY.md](COMPATIBILITY.md) |
| Daemon | Yes | `cmd/reticulum-go`: config, interfaces, transport, identity storage |
| Core stack | Yes | `pkg/transport`, `pkg/packet`, `pkg/destination`, `pkg/announce`, `pkg/pathfinder` |
| Links, resources, channel, buffer | Yes | `pkg/link`, `pkg/resource`, `pkg/channel`, `pkg/buffer` |
| Cryptography | Yes | Centralized in `pkg/cryptography`; details in [docs/cryptography.md](docs/cryptography.md) |
| Identity (software + optional hardware-bound signing) | Yes | `pkg/identity`, `LoadIdentityFile`, `NewIdentityWithSigner`, RHB1 descriptor |
| IFAC (interface access code) | Yes | `pkg/ifac`; masks UDP/TCP/Auto frames per reference |
| Discovery / blackhole | Partial | `pkg/discovery`, `pkg/blackhole` (see compatibility table) |
| Interfaces | Partial | UDP, TCP client/server, Auto, WebSocket (native/WASM); see [COMPATIBILITY.md](COMPATIBILITY.md#interfaces) |
| Interface hot reload | Yes | `ReloadInterfaces`, `SIGHUP` (Unix); not in Python `rns` |
| WASM / browser | Yes | `cmd/reticulum-wasm`, `pkg/wasm` |
| Supply chain secure | Yes | Vendored deps, cosign attestations, CI scans; see [SECURITY.md](SECURITY.md) |

**Goals:**
- Full protocol interoperability with the Python reference implementation
- Portability
- High performance via Go's concurrency model and best coding practices

### Cryptography

Algorithms, key formats, storage, IFAC, and operational guidance are documented in [docs/cryptography.md](docs/cryptography.md). Application code should use `pkg/cryptography` and `pkg/identity` rather than ad hoc primitives.

## Requirements

- Go 1.26.4 or later

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
| `make deps` | Download and verify dependencies (uses the module network; run after editing imports or versions) | `go mod download` and `go mod verify` with the public proxy |
| `make run` | Run with go run | `go run ./cmd/reticulum-go` |
| `make debug` | Build debug binary | `mkdir -p bin` then `go build -o bin/reticulum-go ./cmd/reticulum-go` |
| `make build-linux` | Cross-build for Linux (amd64, arm64, arm, riscv64) | set `GOOS=linux` and `GOARCH=...` per [Makefile](Makefile) |
| `make build-windows` | Cross-build for Windows | set `GOOS=windows` and `GOARCH=...` per [Makefile](Makefile) |
| `make build-windows-legacy` | Cross-build for Windows 7/8/8.1 using [go-legacy-win7](https://github.com/thongtech/go-legacy-win7) | requires go-legacy-win7 on `PATH` or set `GO_LEGACY_WIN7` |
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

### Windows 7, 8, and 8.1

Official Go 1.21 and later no longer support Windows 7. Release builds for legacy Windows use [go-legacy-win7](https://github.com/thongtech/go-legacy-win7), a maintained fork that restores compatibility with Windows 7, 8, 8.1, and Server 2008 R2 through 2012 R2.

Tagged releases include `reticulum-go-windows-amd64-win7.exe` and `reticulum-go-windows-arm64-win7.exe`. These binaries also run on Windows 10 and later.

To build locally, install go-legacy-win7 and run:

```bash
make build-windows-legacy
```

```bash
task build-windows-legacy
```

Or cross-compile with an explicit compiler path:

```bash
GO_LEGACY_WIN7=/usr/local/go-legacy-win7/bin/go make build-windows-legacy
```

CI installs go-legacy-win7 via `scripts/ci/setup-go-legacy-win7.sh`.

## WebAssembly and Embedded

Build WebAssembly binary (requires Task):

```bash
task build-wasm
task test-wasm
```

For embedded systems and TinyGo builds, see the `tinygo` branch in your Reticulum-Go checkout. Requires TinyGo 0.41.0+.

## Vendored dependencies and offline builds

### Why we vendor

Vendoring keeps the exact third-party source tree in this repository so builds and tests do not depend on fetching modules at compile time. That supports air-gapped and offline environments, avoids coupling releases to the availability of public module proxies or hosting sites, and makes the dependency set easy to review in diffs and audits. It is also central to supply chain security for dependencies: ordinary builds compile what is committed here, not whatever a proxy or upstream source might serve at build time, and changes to third-party code show up in review as normal source diffs. Dependency versions are still recorded in `go.mod` and `go.sum`; `vendor/` is the canonical copy used for ordinary builds.

The Makefile and Taskfile default to `GOFLAGS=-mod=vendor` and `GOPROXY=off`, so a normal `make build`, `make test`, or `task build` / `task test` does not contact module proxies, the checksum database, or Git remotes for dependencies. Only the Go toolchain (and the standard library it ships with) is required besides this repository.

CI sets the same variables for build, test, and related jobs. Steps that install standalone tools with `go install` (for example revive, gosec, and govulncheck in `scripts/ci/`) temporarily clear those flags so the installer can fetch those binaries; project code still builds from `vendor/`.

When you change first-party libraries under `Reticulum-Go-Projects/`, run `task vendor-sync` (or `make deps`) with `LIBS_ROOT` pointing at that tree. That refreshes `go.mod` replace paths and regenerates `vendor/` for the root module and the `examples/wasm` and `examples/pageserver` modules. Commit `go.mod`, `go.sum`, and the updated `vendor/` trees. Ordinary clones only need `vendor/` to build offline; the sibling lib checkout is required for re-vendoring, not for day-to-day builds.

The `examples/wasm` and `examples/pageserver` trees have their own `go.mod` and `vendor/` directories. Docker images under `docker/` copy `vendor/` and build with the same offline module settings.

## Credit

[Mark Qvist](https://github.com/markqvist) - For creating the Reticulum Network Stack

## License

Apache License 2.0. See [LICENSE](LICENSE).
