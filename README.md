# Reticulum-Go

A high-performance and [secure](SECURITY.md) Go implementation of the [Reticulum Network Stack](https://github.com/markqvist/Reticulum).

## Overview

Reticulum-Go provides full protocol compatibility with the Python reference implementation while leveraging Go's concurrency model for improved throughput and latency. The implementation targets cross-platform deployment across legacy and modern systems.

See [COMPATIBILITY.md](COMPATIBILITY.md) for how this is verified against the Python stack and the [network API reference](https://reticulum.network/manual/reference.html).

Full documentation (English): [docs/en/](docs/en/README.md). Application authors: [API reference](docs/en/api-reference.md) and [Examples](docs/en/examples.md). Additional languages will live alongside `docs/en/` when translated.

## Features

| Area | Status | Notes |
|------|:------:|-------|
| Wire compatibility with Python [Reticulum](https://github.com/markqvist/Reticulum) | Yes | Packet and crypto paths cross-checked (`tests/crossref`, interop tests). See [COMPATIBILITY.md](COMPATIBILITY.md) |
| Daemon and tools | Yes | Single `reticulum-go` binary: daemon (default), `status`, `id`, `probe`, `path`, `cp`, `x`, `pageserver` |
| Core stack | Yes | `pkg/transport`, `pkg/packet`, `pkg/destination`, `pkg/announce`, `pkg/pathfinder` |
| Links, resources, channel, buffer | Yes | `pkg/link`, `pkg/resource`, `pkg/channel`, `pkg/buffer` |
| Cryptography | Yes | Centralized in `pkg/cryptography`. Details in [docs/en/cryptography.md](docs/en/cryptography.md) |
| Identity (software + optional hardware-bound signing) | Yes | `pkg/identity`, `LoadIdentityFile`, `NewIdentityWithSigner`, RHB1 descriptor |
| IFAC (interface access code) | Yes | `pkg/ifac`, masks UDP/TCP/Auto frames per reference |
| Discovery / blackhole | Partial | `pkg/discovery`, `pkg/blackhole` (see compatibility table) |
| Interfaces | Partial | UDP, TCP, Auto, Pipe, Local, WebSocket (native/WASM), QUIC (native). See [COMPATIBILITY.md](COMPATIBILITY.md#interfaces) |
| Interface hot reload | Yes | `ReloadInterfaces`, `SIGHUP` (Unix), not in Python `rns` |
| WASM / browser | Yes | `cmd/reticulum-wasm`, `pkg/wasm` |
| Runtime sandbox | Yes | `pkg/sandbox`, enabled by default. See [SECURITY.md](SECURITY.md#runtime-sandbox) |
| librns C ABI | Yes | Linux shared library for in-process embed (`include/rns.h`, `task build-librns`). See [docs/en/librns.md](docs/en/librns.md) |
| Control API | Yes | Localhost JSON and WebSocket for out-of-process clients. See [docs/en/control-api.md](docs/en/control-api.md) |
| CLI utilities | Yes | Subcommands of `reticulum-go` (legacy `rgo*` names install as symlinks). See [docs/en/utilities.md](docs/en/utilities.md) |
| Supply chain secure | Yes | Vendored deps, cosign attestations, CI scans. See [SECURITY.md](SECURITY.md) |

**Goals:**
- Full protocol interoperability with the Python reference implementation
- Portability
- High performance via Go's concurrency model and best coding practices

### Cryptography

Algorithms, key formats, storage, IFAC, and operational guidance are documented in [docs/en/cryptography.md](docs/en/cryptography.md). Application code should use `pkg/cryptography` and `pkg/identity` rather than ad hoc primitives.

### Runtime sandbox

The `reticulum-go` daemon applies a platform-specific sandbox after startup (`pkg/sandbox`). It is **on by default** and can be turned off in config:

```ini
enable_sandbox = no
```

Linux uses Landlock (kernel 5.13+) to whitelist paths the daemon needs. OpenBSD uses `unveil` and `pledge`. FreeBSD uses capability mode. Windows applies job-object limits. Details and limitations are in [SECURITY.md](SECURITY.md#runtime-sandbox).

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

Install the binary, legacy tool symlinks (`rgostatus`, `rgoid`, …), and man pages (default prefix `/usr/local`):

```bash
make install
```

Custom prefix:

```bash
make install PREFIX=/opt/reticulum
```

Staging for packaging:

```bash
make install DESTDIR=/tmp/stage PREFIX=/usr
```

Alternatively, install into your Go toolchain binary directory (`$GOBIN` or `$(go env GOPATH)/bin`):

```bash
CGO_ENABLED=0 go install -ldflags="-s -w" ./cmd/reticulum-go
```

### Packages

Build `.deb` / `.rpm` with [nfpm](https://nfpm.goreleaser.com/) (fetched on demand):

```bash
make package-deb
make package-rpm
```

Artifacts land in `dist/`. Config: [packaging/nfpm.yaml](packaging/nfpm.yaml).

### Usage

```bash
reticulum-go                  # daemon
reticulum-go status           # interface stats (RPC)
reticulum-go id -h
reticulum-go probe ...
reticulum-go path -t
reticulum-go cp -l
reticulum-go x -l
reticulum-go pageserver
```

Man pages: `man reticulum-go`, `man 8 reticulum-go`, `man reticulum-go-status`, …

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
| `make build` | Build release binary (stripped, static) | `CGO_ENABLED=0 go build … -o bin/reticulum-go ./cmd/reticulum-go` |
| `make build-utils` | Alias for `build` (tools are subcommands) | see [docs/en/utilities.md](docs/en/utilities.md) |
| `make install` | Binary, tool symlinks, and man pages under PREFIX | supports `DESTDIR` |
| `make install-man` | Man pages only | `man/*.1` and `man/*.8` |
| `make package-deb` / `package-rpm` | Linux packages via nfpm | output in `dist/` |
| `make uninstall` | Remove binary, symlinks, and man pages | |
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
| `make deps` | Download and verify dependencies (uses the module network, run after editing imports or versions) | `go mod download` and `go mod verify` with the public proxy |
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

Note: On some systems, use `go-task` instead of `task`. Add `alias task='go-task'` to your shell config if needed.

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

Cross-compilation uses `GOOS` and `GOARCH` with the same `go build` flags as `make build`. See [Makefile](Makefile) `build-linux`, `build-windows`, and `build-darwin` targets for exact commands.

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

CI uses pinned Go **1.26.5** via `actions/setup-go` in `.github/actions/setup-ci`. Legacy Windows builds use `scripts/ci/setup-go-legacy-win7.sh` (SHA256-pinned release tarball).

## WebAssembly and Embedded

Build WebAssembly binary (requires Task):

```bash
task build-wasm
task test-wasm
```

### librns (C ABI)

In-process embed for C, C++, and similar FFI hosts. The daemon stays `CGO_ENABLED=0`. Only this target needs CGO.

```bash
task build-librns
make -C examples/librns-smoke
./examples/librns-smoke/librns-smoke
```

Outputs `bin/librns.so` and `include/rns.h`. Full map: [docs/en/librns.md](docs/en/librns.md).

For TinyGo and very small devices, see the `tinygo` branch. Requires TinyGo 0.41.0 or newer.

## Vendored dependencies and offline builds

### Why we vendor

Vendoring keeps the exact third-party source tree in this repository so builds and tests do not depend on fetching modules at compile time. That supports air-gapped and offline environments, avoids coupling releases to the availability of public module proxies or hosting sites, and makes the dependency set easy to review in diffs and audits. It is also central to supply chain security for dependencies: ordinary builds compile what is committed here, not whatever a proxy or upstream source might serve at build time, and changes to third-party code show up in review as normal source diffs. Dependency versions are still recorded in `go.mod` and `go.sum`. `vendor/` is the canonical copy used for ordinary builds.

The Makefile and Taskfile default to `GOFLAGS=-mod=vendor` and `GOPROXY=off`, so a normal `make build`, `make test`, or `task build` / `task test` does not contact module proxies, the checksum database, or Git remotes for dependencies. Only the Go toolchain (and the standard library it ships with) is required besides this repository.

CI sets the same variables for build, test, and related jobs. Steps that install standalone tools with `go install` (for example revive, gosec, and govulncheck in `scripts/ci/`) temporarily clear those flags so the installer can fetch those binaries. Project code still builds from `vendor/`.

When you change first-party libraries under `Reticulum-Go-Projects/`, run `task vendor-sync` (or `make deps`) with `LIBS_ROOT` pointing at that tree. That refreshes `go.mod` replace paths and regenerates `vendor/` for the root module and the `examples/wasm` and `examples/pageserver` modules. Commit `go.mod`, `go.sum`, and the updated `vendor/` trees. Ordinary clones only need `vendor/` to build offline. The sibling lib checkout is required for re-vendoring, not for day-to-day builds.

The `examples/wasm` and `examples/pageserver` trees have their own `go.mod` and `vendor/` directories. Docker images under `docker/` copy `vendor/` and build with the same offline module settings.

## Credit

[Mark Qvist](https://github.com/markqvist) - For creating the Reticulum Network Stack

## License

Apache License 2.0. See [LICENSE](LICENSE).
