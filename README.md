# Reticulum-Go

Reticulum-Go is a high-performance and [secure](SECURITY.md) Golang implementation of the [Reticulum Network Stack](https://github.com/markqvist/Reticulum). It is intended to strengthen the existing networks and bring Reticulum to more devices.

Available on rngit:

NomadNet Node: `132f67e79d9b24aad014e93015fb858f:/page/index.mu`
`git clone rns://06a54b505bb67b25ef3f8097e8001edc/public/Reticulum-Go`

## Overview

Reticulum-Go provides full protocol compatibility with the Python reference implementation. It leverages the Go concurrency model to deliver improved throughput and lower latency. The implementation is designed for cross-platform deployment across both modern and legacy systems.

For details on interoperability, see [COMPATIBILITY.md](COMPATIBILITY.md). You can find the canonical network API reference at the [Reticulum Manual](https://reticulum.network/manual/reference.html).

### Documentation Index

*   **Full English Documentation:** [docs/en/](docs/en/README.md)
*   **For Application Authors:** [API Reference](docs/en/api-reference.md) and [Examples](docs/en/examples.md)
*   **Additional Languages:** Translations will live alongside `docs/en/` as they become available.

### Main Goals

*   Excellent portability and support for legacy operating systems
*   Clear auditability and supply chain security
*   Full protocol interoperability with the Python reference implementation and its standard utilities
*   High performance using modern Go concurrency patterns and optimized code
*   High security with native sandboxing and firecracker microvm support
*   Reliable operation with reconnect, NIC watching, and interface hot reload

## Features

| Functional Area | Implementation Status | Notes |
| :--- | :---: | :--- |
| **Protocol Compatibility** | Yes | Full wire compatibility with the Python reference. Checked using cross-reference and interop tests (`tests/crossref`). See [COMPATIBILITY.md](COMPATIBILITY.md). |
| **Daemon and Utilities** | Yes | Provided via a single `reticulum-go` binary containing the daemon and utility subcommands. |
| **Core Network Stack** | Yes | Includes packet processing, transport, destination management, announce handling, and pathfinding. |
| **Links and Channels** | Yes | Full support for links, resources, channels, and buffers (`pkg/link`, `pkg/resource`, `pkg/channel`, `pkg/buffer`). |
| **Cryptography** | Yes | Centralized in `pkg/cryptography`. See [docs/en/cryptography.md](docs/en/cryptography.md) for full details. |
| **Identity Management** | Yes | Software identities and hardware-bound signing (using the RHB1 descriptor). |
| **Interface Access Codes (IFAC)** | Yes | Masking support for UDP, TCP, and Auto frames matching the Python reference implementation. |
| **Discovery and Blackholing** | Partial | Partial support implemented in `pkg/discovery` and `pkg/blackhole`. Refer to the compatibility table. |
| **Network Interfaces** | Partial | UDP, TCP, Auto, Pipe, Local, Serial, WebSocket, QUIC, WebTransport, DNS rendezvous, VSOCK (Linux), HTTPS long-poll. See [COMPATIBILITY.md](COMPATIBILITY.md#interfaces) and [docs/en/interfaces.md](docs/en/interfaces.md). |
| **Interface Hot Reload** | Yes | Support for interface reloading using `ReloadInterfaces` or via a `SIGHUP` signal on Unix. This feature is not present in Python `rns`. |
| **WebAssembly Support** | Yes | Run in the browser or WebAssembly environments (`cmd/reticulum-wasm`, `pkg/wasm`). |
| **Runtime Sandbox** | Yes | Automated OS-level sandboxing enabled by default. See [SECURITY.md](SECURITY.md#runtime-sandbox). |
| **librns C ABI** | Yes | Linux shared library for embedded applications. See [docs/en/librns.md](docs/en/librns.md). |
| **Odin librns bindings** | Yes | Idiomatic Odin package over `librns.so` (`bindings/odin`). Linux. See [docs/en/librns.md](docs/en/librns.md#odin-bindings). |
| **Dart librns FFI** | Yes | In-process `dart:ffi` over `librns` on Linux, Android, and Windows (`bindings/dart`, `package:rns_control/ffi.dart`). See [docs/en/librns.md](docs/en/librns.md#dart-ffi-bindings). |
| **Dart Control API client** | Yes | Flutter-ready Dart package over the Control API (`bindings/dart`). See [docs/en/control-api.md](docs/en/control-api.md#dart-and-flutter). |
| **Control API** | Yes | Localhost JSON and WebSocket APIs for out-of-process clients. See [docs/en/control-api.md](docs/en/control-api.md). |
| **CLI Utilities** | Yes | Native subcommands of `reticulum-go`. Installs symlinks to match legacy `rgo*` tool names. See [docs/en/utilities.md](docs/en/utilities.md). |
| **Supply Chain Security** | Yes | Protected by fully vendored dependencies, cosign release attestations, and automated CI security scans. See [SECURITY.md](SECURITY.md). |

### Cryptography

Algorithms, key formats, storage, IFAC, and operational guidance are documented in [docs/en/cryptography.md](docs/en/cryptography.md). Application code should use `pkg/cryptography` and `pkg/identity` rather than writing ad hoc cryptographic operations.

### Runtime Sandbox

The `reticulum-go` daemon applies a platform-specific sandbox after startup. The sandbox is **on by default**. You can disable it by adding this option to your configuration file:

```ini
enable_sandbox = no
```

Depending on your platform, the sandbox uses different isolation mechanisms:
*   **Linux:** Landlock (kernel 5.13+) restricts filesystem access to the folders the daemon needs, then a soft-fail seccomp-bpf denylist blocks high-risk syscalls.
*   **OpenBSD:** Uses `unveil` and `pledge` to limit system and file operations.
*   **FreeBSD:** Enters capability mode to isolate the process.
*   **Windows:** Applies job-object limits to control system resources.

Refer to [SECURITY.md](SECURITY.md#runtime-sandbox) for details and specific platform limitations.

## Requirements

*   Go version 1.26.5 or later
*   Optional for Odin bindings: Odin compiler on `PATH` (CI pins `dev-2026-06`), plus CGO to build `librns.so`
*   Optional for Dart / Flutter bindings: Dart SDK on `PATH` (CI pins `3.11.4`). FFI tests also need CGO to build `librns.so`

## Quick Start

You can use the provided [Makefile](Makefile) targets or run the equivalent `go` commands directly if you do not have Make installed.

### Build

Compile the native binary:

```bash
make build
```

Or build manually using Go:

```bash
mkdir -p bin
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reticulum-go ./cmd/reticulum-go
```

The output binary will be created at `bin/reticulum-go`.

### Install

Install the main binary, legacy tool symlinks (such as `rgostatus` and `rgoid`), and man pages to the default prefix `/usr/local`:

```bash
make install
```

To use a custom installation directory:

```bash
make install PREFIX=/opt/reticulum
```

To stage files for package managers:

```bash
make install DESTDIR=/tmp/stage PREFIX=/usr
```

### Install as a service

Install init service files for the detected init system (or set `INIT`):

```bash
make install-service
make install-service INIT=systemd
make install-service INIT=openrc
make install-service INIT=runit
make install-service INIT=dinit
make install-service INIT=all
```

Service units live under [packaging/](packaging/) (`systemd`, `openrc`, `runit`, `dinit`). The install creates `/var/lib/reticulum-go/` with a sample config that logs to both stderr and `/var/lib/reticulum-go/logfile/reticulum.log`.

Alternatively, install directly into your Go binary directory:

```bash
CGO_ENABLED=0 go install -ldflags="-s -w" ./cmd/reticulum-go
```

### Packaging

Build `.deb`, `.rpm`, or `.pkg.tar.zst` packages using [nfpm](https://nfpm.goreleaser.com/). The tool is fetched on demand:

```bash
make package-deb
make package-rpm
make package-arch
```

The package files are placed in the `dist/` folder. The packaging options are configured in [packaging/nfpm.yaml](packaging/nfpm.yaml).

### Command Usage

Run the main daemon or query status and paths:

```bash
reticulum-go                  # Starts the background daemon
reticulum-go status           # Displays interface statistics
reticulum-go slow             # Bottleneck and local health findings
reticulum-go id -h            # Shows identity options
reticulum-go probe ...        # Proves path reachability
reticulum-go path -t          # Inspects known paths
reticulum-go cp -l            # Handles copy operations
reticulum-go x -l             # Executes remote commands
reticulum-go pageserver       # Runs the built-in page server
```

You can view the documentation by running manual commands:
*   `man reticulum-go`
*   `man 8 reticulum-go`
*   `man reticulum-go-status`
*   `man reticulum-go-slow`
### Run from Source

Run the daemon directly from the source code:

```bash
make run
```

Or run manually:

```bash
go run ./cmd/reticulum-go
```

### Run Tests

Run the full test suite:

```bash
make test
```

Host OS preflight (sandbox, interfaces, short daemon):

```bash
make test-self-check
```

Or run manually:

```bash
go test -v ./...
```

## Makefile Reference

| Target | Description | Equivalent Command |
| :--- | :--- | :--- |
| `make` / `make all` | Compiles the release binary. | Matches `make build`. |
| `make build` | Compiles a stripped, static release binary. | `CGO_ENABLED=0 go build ... -o bin/reticulum-go ./cmd/reticulum-go` |
| `make build-utils` | Alias for building the main binary. | Subcommands provide all utilities. See [docs/en/utilities.md](docs/en/utilities.md). |
| `make install` | Installs the binary, legacy symlinks, and man pages. | Installs files under the specified `PREFIX`. Supports `DESTDIR`. |
| `make install-man` | Installs only the manual pages. | Installs `man/*.1` and `man/*.8` files. |
| `make install-service` | Installs systemd, openrc, runit, and/or dinit service files. | `INIT=auto|systemd|openrc|runit|dinit|all`. Sample config under `/var/lib/reticulum-go`. |
| `make package-deb` | Builds a Debian package. | Output is placed in `dist/`. |
| `make package-rpm` | Builds an RPM package. | Output is placed in `dist/`. |
| `make package-arch` | Builds an Arch Linux package. | Output is placed in `dist/`. |
| `make uninstall` | Removes the binary, symlinks, and man pages. | Deletes files from the installation prefix. |
| `make clean` | Deletes build and test artifacts. | Runs `go clean` and removes the `bin` directory. |
| `make test` | Runs the full test suite. | Runs `go test -v ./...` |
| `make test-short` | Runs only short unit tests. | Runs `go test -short -v ./...` |
| `make test-race` | Runs tests with the Go race detector enabled. | Runs `go test -race -v ./...` |
| `make test-services` | Docker tests for logfile + systemd/openrc/runit/dinit services. | Runs `scripts/ci/test-services-docker.sh`. |
| `make test-self-check` | Host OS preflight for reticulum-go platform features. | Runs `scripts/ci/run-self-check.sh`. |
| `make test-self-check-386` | Linux 386 self-check under qemu-user. | Runs `scripts/ci/run-qemu-arch-self-check.sh 386` (needs `qemu-user-static`). |
| `make test-self-check-arm` | Linux arm (GOARM=6) self-check under qemu-user. | Runs `scripts/ci/run-qemu-arch-self-check.sh arm` (needs `qemu-user-static`). |
| `make test-self-check-riscv64` | Linux riscv64 self-check under qemu-user. | Runs `scripts/ci/run-qemu-arch-self-check.sh riscv64` (needs `qemu-user-static`). |
| `make test-self-check-ppc64le` | Linux ppc64le self-check under qemu-user. | Runs `scripts/ci/run-qemu-arch-self-check.sh ppc64le` (needs `qemu-user-static`). |
| `make test-self-check-ppc64` | Linux ppc64 (big-endian) self-check under qemu-user. | Runs `scripts/ci/run-qemu-arch-self-check.sh ppc64` (needs `qemu-user-static`). |
| `make coverage` | Generates and opens a test coverage report. | Runs `go test -coverprofile=coverage.out ./...` and opens it in your browser. |
| `make bench` | Runs all benchmark tests. | Runs `go test -run=^$ -bench=. -benchmem ./...` |
| `make fmt` | Formats all Go source files. | Runs `go fmt ./...` |
| `make vet` | Runs the standard Go vet tool. | Runs `go vet ./...` |
| `make lint` | Runs the revive linter. | Runs `revive -config revive.toml -formatter friendly ./pkg/* ./cmd/* ./internal/*` |
| `make vulncheck` | Runs govulncheck. | Runs `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...` |
| `make check` | Runs formatting, vetting, linting, short tests, and vulncheck. | Runs all code quality checks in sequence. |
| `make deps` | Downloads and verifies Go modules. | Runs `go mod download` and `go mod verify` using the public proxy. |
| `make run` | Compiles and runs the daemon. | Runs `go run ./cmd/reticulum-go` |
| `make debug` | Compiles a standard debug binary with symbols. | Runs `go build -o bin/reticulum-go ./cmd/reticulum-go` |
| `make microvm-up` | Prepare and start Firecracker microvm + host bridge. | Runs `./microvm/up.sh`. See [docs/en/microvm.md](docs/en/microvm.md). |
| `make microvm-stop` | Stop Firecracker microvm and host bridge. | Runs `./microvm/stop.sh`. |
| `make build-linux` | Cross-compiles for Linux. | Cross-compiles for amd64, arm64, arm, 386, riscv64, ppc64le, and ppc64 targets. |
| `make build-windows` | Cross-compiles for Windows. | Cross-compiles for amd64 and arm64 targets. |
| `make build-windows-legacy` | Cross-compiles for legacy Windows releases. | Compiles Windows 7, 8, and 8.1 support using go-legacy-win7. |
| `make build-darwin` | Cross-compiles for macOS. | Cross-compiles for amd64 and arm64 targets. |
| `make build-all` | Cross-compiles for all major platforms. | Compiles Linux, Windows, and macOS binaries. |
| `make tree-rsm-verify` | Verifies `reticulum-go.rsm` signature and file hashes. | `sh scripts/ci/verify-tree-rsm.sh` |
| `make tree-rsm-sign` | Signs the tree inventory into `reticulum-go.rsm`. | Requires `RNS_ID_PATH`. See [SECURITY.md](SECURITY.md). |
| `make hooks-install` | Enables the tracked pre-commit hook that resigns the RSM. | `sh scripts/ci/install-git-hooks.sh` |

## Taskfile Automation

The project provides a [Taskfile](https://taskfile.dev/) for advanced local automation. If you have Task installed, run `task --list` to view all available tasks.

```bash
task build
task install
task test
```

On some Linux distributions, the command is named `go-task` instead of `task`. You can add an alias to your shell profile if needed:

```bash
alias task='go-task'
```

## Development and Contributions

### Code Quality Checks

Run the verification suite before submitting pull requests:

```bash
make fmt
make vet
make lint
make check
```

These targets format code, run static analysis, check for common linter issues, and run short tests to ensure regressions are not introduced.

### Cross-Platform Builds

Verify cross-compilation across platforms:

```bash
make build-linux
make build-windows
make build-darwin
make build-all
```

Cross-compilation uses the same optimization flags as native builds. See the [Makefile](Makefile) targets for exact environmental variables.

### Windows 7, 8, and 8.1 Support

Official Go 1.21 and newer releases no longer support Windows 7. To support legacy deployments, we compile our legacy Windows releases using [go-legacy-win7](https://github.com/thongtech/go-legacy-win7). This maintained fork restores compatibility with Windows 7, 8, 8.1, Server 2008 R2, and Server 2012 R2.

Official tagged releases include `reticulum-go-windows-amd64-win7.exe` and `reticulum-go-windows-arm64-win7.exe` binaries. These files run on both modern and legacy Windows installations.

To compile legacy Windows binaries locally, install go-legacy-win7 and run:

```bash
make build-windows-legacy
```

Or build using Task:

```bash
task build-windows-legacy
```

You can also specify a custom path to your legacy Go compiler:

```bash
GO_LEGACY_WIN7=/usr/local/go-legacy-win7/bin/go make build-windows-legacy
```

CI uses a pinned Go **1.26.5** compiler via GitHub Actions. Legacy Windows builds use `scripts/ci/setup-go-legacy-win7.sh` to download and verify the legacy compiler.

## WebAssembly and Embedded Targets

To compile the WebAssembly binary, run the following Task commands:

```bash
task build-wasm
task test-wasm
```

### librns Shared Library

For in-process embedding in C, C++, and Python (FFI), we provide a C ABI target. The main daemon is built with `CGO_ENABLED=0`, but the shared library compilation requires CGO.

To build the shared library:

```bash
task build-librns
make -C examples/librns-smoke
./examples/librns-smoke/librns-smoke
```

This compiles the shared library to `bin/librns.so` and generates header definitions at `include/rns.h`. For details, see [docs/en/librns.md](docs/en/librns.md).

### Odin Bindings

Idiomatic Odin wrappers over `librns.so` live in `bindings/odin`. Requires the Odin compiler on `PATH`.

```bash
task build-librns
task test-odin
```

Covers node lifecycle, identity, destinations, paths, links, and events. Details: [docs/en/librns.md](docs/en/librns.md#odin-bindings).

### Dart / Flutter Bindings

Package `rns_control` in `bindings/dart` provides:

- In-process librns FFI (`package:rns_control/ffi.dart`) on Linux, Android, and Windows
- Control API HTTP and WebSocket client for a running daemon

```bash
task build-librns
task test-dart
# cross-build native libraries
task build-librns-targets -- linux android windows
```

Details: [docs/en/librns.md](docs/en/librns.md#dart-ffi-bindings) and [docs/en/control-api.md](docs/en/control-api.md#dart-and-flutter).

If you are compiling for TinyGo and small microcontroller boards, check out our `tinygo` branch. This requires TinyGo version 0.41.0 or newer.

## Vendored Dependencies and Offline Builds

### Why We Vendor Dependencies

We fully vendor all third-party dependencies. The source trees are stored inside the `vendor/` directory of this repository. This approach provides several key benefits:

*   **Build Reliability:** Compilation and testing do not depend on downloading external modules. This allows you to build the project in completely air-gapped or offline environments.
*   **Release Stability:** Release builds do not rely on the public availability of module proxies or upstream hosting platforms.
*   **Auditability and Security:** Every dependency change or upgrade appears as a standard file diff in pull requests. This makes it straightforward to inspect third-party changes during code audits.
*   **Supply Chain Control:** Normal builds compile the exact source code committed in the repository, protecting you against upstream dependency attacks.

Versions and checksums are still officially tracked inside the `go.mod` and `go.sum` files. The `vendor/` directories serve as the canonical source copy for our compilers.

### Offline Build Configurations

Both the [Makefile](Makefile) and [Taskfile](Taskfile.yaml) automatically set `GOFLAGS=-mod=vendor` and `GOPROXY=off`. Standard compilation and testing commands will not contact external networks.

The continuous integration pipeline uses these identical flags. The only exception is scripts that install standalone CLI tools, such as `revive` or `gosec` in our setup scripts. These temporary installation tasks temporarily clear the environment flags to fetch the binary tools, but the actual project code is always compiled from the local `vendor/` folder.

### Synchronizing Sibling Libraries

If you are developing first-party libraries under `Reticulum-Go-Projects/`, you can synchronize imports using:

```bash
task vendor-sync
```

Alternatively, run:

```bash
make deps
```

Set the `LIBS_ROOT` environment variable to point to your local libraries repository. This command will update the replace paths in `go.mod` and update the vendor folders. Make sure to commit the updated `go.mod`, `go.sum`, and updated `vendor/` trees.

Ordinary source checkouts only need the `vendor/` directories to build offline. Sibling repository checkouts are only required if you are actively updating or re-vendoring first-party libraries.

The `examples/wasm` and `examples/pageserver` examples contain their own independent `go.mod` and `vendor/` files. The Docker configurations under `docker/` copy these folders to build matching containers offline.

## AI Disclaimer

Open-weight LLMs, preferably operated locally under a controlled harness, may assist with non-critical tasks such as commit messages, documentation, drafts, translations, and tests. LLMs are strictly excluded from cryptography, key handling, protocol security logic, and any other security-sensitive development. All LLM output is reviewed and approved by a human. Design decisions and security-critical changes are made exclusively by humans.


## Credit

*   [Mark Qvist](https://github.com/markqvist) for designing and implementing the reference Reticulum Network Stack.

## License

This project is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for the full text.
