# Getting started

## Requirements

- Go 1.26.5 or later
- Make or Task (optional, for convenience targets)
- A writable home directory for `~/.reticulum-go`

The repository vendors dependencies. A normal build does not contact module proxies when `GOFLAGS=-mod=vendor` is set (default in the Makefile and Taskfile).

## Build

From the repository root:

```bash
make build
```

This produces `bin/reticulum-go` as a static stripped binary (`CGO_ENABLED=0`) with the daemon and all tools as subcommands.

Equivalent manual command:

```bash
mkdir -p bin
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reticulum-go ./cmd/reticulum-go
```

## Install to PATH

```bash
make install
```

Default prefix is `/usr/local`. That installs `reticulum-go`, legacy tool symlinks (rgostatus, rgoid, …), and man pages (reticulum-go(1), reticulum-go(8), and tool pages). Override with `make install PREFIX=/opt/reticulum`. Staging: `make install DESTDIR=/tmp/stage PREFIX=/usr`.

Or install into your Go binary directory:

```bash
CGO_ENABLED=0 go install -ldflags="-s -w" ./cmd/reticulum-go
```

Linux packages:

```bash
make package-deb
make package-rpm
make package-arch
```

Arch Linux and CachyOS: add the Quad4 pacman repo from [quad4-arch](https://github.com/Quad4-Software/quad4-arch) (`reticulum-go` or `reticulum-go-git`). That is a Quad4-hosted repo, not AUR.

## First run

```bash
make run
```

Or:

```bash
go run ./cmd/reticulum-go
```

On first start the daemon creates `~/.reticulum-go/` with a default config if none exists. Logs go to stderr by default. Set verbosity with `[logging] loglevel` (1 through 7). Set `[logging] destination = file|both` and optional logfile to write to disk (default `{config_dir}/logfile/reticulum.log`). Daemon text logs, pageserver banner, and CLI tools color on TTY. Respect `NO_COLOR` and `FORCE_COLOR` / `CLICOLOR_FORCE`.

Daemon flags:

```bash
reticulum-go --config ~/.reticulum-go/config -debug 5
reticulum-go --config /path/to/config-dir
```

### Custom config path

Pass `--config` / `-config` with a config file or directory (directory uses config inside it).

## Minimal configuration

A useful starting point enables transport and one UDP interface to a known peer:

```ini
[reticulum]
enable_transport = yes
share_instance = yes

[logging]
loglevel = 4

[[UDP Peer]]
type = UDPInterface
enabled = yes
interface_enabled = yes
target_address = 192.0.2.10
target_port = 4242
port = 4242
```

UDP requires an explicit target_address or target_host. Open binds do not learn peers from the first inbound packet (same policy as Python forward_ip).

For local mesh discovery over IPv6 link-local multicast, use AutoInterface. See [Interfaces](interfaces.md).

## Verify the build

```bash
make test-short
```

Full test suite:

```bash
make test
```

Cross-reference tests against Python vectors (requires Python 3 and vector generation):

```bash
./tests/crossref/run_crossref.sh all
```

## Cross-platform builds

```bash
make build-linux
make build-windows
make build-darwin
make build-all
```

Legacy Windows 7, 8, and 8.1 builds use [go-legacy-win7](https://github.com/thongtech/go-legacy-win7):

```bash
make build-windows-legacy
```

## WebAssembly

Install [Task](https://taskfile.dev/) and run:

```bash
task build-wasm
task test-wasm
```

See [Embedding and WebAssembly](embedding-and-wasm.md).

## librns and language bindings

Build the shared library and optional binding tests:

```bash
task build-librns
make -C bindings/c/examples/smoke && ./bindings/c/examples/smoke/librns-smoke
task test-odin
task test-zig
task test-cpp
```

`task test-odin` needs the Odin compiler on `PATH`. `task test-zig` needs Zig 0.16.0 or later on `PATH`. `task test-cpp` needs CMake and a C++17 compiler. See [librns](librns.md).

## Dart bindings

```bash
task build-librns
task test-dart
```

Needs the Dart SDK on `PATH`. FFI uses librns on Linux, Android, and Windows. See [librns](librns.md#dart-ffi-bindings) and [Control API](control-api.md#dart-and-flutter).

## Enable the control API

Add to `[reticulum]`:

```ini
enable_control_api = yes
rpc_key = <64 hex characters>
control_api_host = 127.0.0.1
control_api_port = 37430
```

Generate a random 32-byte key and encode as hex. Clients send `Authorization: Bearer <rpc_key>`. See [Control API](control-api.md).

## CLI utilities (status, identity, probe, path, copy, pageserver)

Tools are subcommands of the single `reticulum-go` binary (`make build`). Legacy names (rgostatus, …) install as symlinks via `make install`.

To query a running Python rnsd from `reticulum-go status` / path, point `-config` at `~/.reticulum`. On Linux both stacks default to abstract Unix sockets when shared_instance_type is unset, so no TCP rewrite is required:

```bash
./bin/reticulum-go status -config ~/.reticulum -json
./bin/reticulum-go path -config ~/.reticulum -t -json
```

Prefer an explicit shared rpc_key when mixing stacks. Use `shared_instance_type = tcp` only when you want the same recipe on every OS.

Full flag reference, `.rsg` / `.rsm` / `.rfe` usage, file transfer, and troubleshooting are in [CLI utilities](utilities.md).

## Disable the sandbox

Sandboxing is on by default. To turn it off (not recommended for production):

```ini
enable_sandbox = no
```

See [Security](security.md) for platform behavior.

## Troubleshooting

**Daemon exits on config error.** Check the config path and syntax. Unknown keys are ignored so a damaged file can still boot. Fix typos in type and interface names.

**No paths to remote destinations.** Confirm interfaces are enabled, peers are reachable, and transport is enabled. Use debug level 5 or higher temporarily. Request paths explicitly from application code or the control API.

**IFAC mismatches.** Peers must use the same network_name and passphrase. Wrong IFAC frames are dropped silently on ingress.

**Shared instance conflicts.** Only one process should own interfaces when `share_instance = yes`. Others should connect as clients. Check shared_instance_port (default 37428).

**status connection refused.** Point `-config` at the daemon config dir (`~/.reticulum` for rnsd). Align shared_instance_type and instance_name / ports, or leave the type unset on Linux for Unix. See [CLI utilities](utilities.md).

**Permission errors on Linux sandbox.** Landlock requires kernel 5.13+. The config directory and storage paths must live under whitelisted locations. See [Security](security.md).

## Next steps

| Goal | Document |
|------|----------|
| Configure interfaces and rates | [Configuration](configuration.md), [Interfaces](interfaces.md) |
| Status / identity / probe / path / copy CLIs | [CLI utilities](utilities.md) |
| Write a Go app | [API reference](api-reference.md), [Examples](examples.md), [Embedding and WebAssembly](embedding-and-wasm.md) |
| Embed from C or Odin | [librns](librns.md), [Examples](examples.md) |
| Flutter / Dart | [librns Dart FFI](librns.md#dart-ffi-bindings), [Control API](control-api.md#dart-and-flutter), [Examples](examples.md) |
| Talk to a running daemon | [Control API](control-api.md) |
| Run in Firecracker | [Firecracker microvm](microvm.md) (`make microvm-up`) |
| Use Python interop | [Compatibility](compatibility.md) |
| Run examples | [Examples](examples.md) |
