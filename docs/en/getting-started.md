# Getting started

| Field | Value |
|-------|-------|
| Document version | 1.0 |
| Last updated | 2026-07-07 |
| Author | Ivan |

## Requirements

- Go 1.26.4 or later
- Make or Task (optional, for convenience targets)
- A writable home directory for `~/.reticulum-go`

The repository vendors dependencies. A normal build does not contact module proxies when `GOFLAGS=-mod=vendor` is set (default in the Makefile and Taskfile).

## Build the daemon

From the repository root:

```bash
make build
```

This produces `bin/reticulum-go` as a static stripped binary (`CGO_ENABLED=0`).

Equivalent manual command:

```bash
mkdir -p bin
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reticulum-go ./cmd/reticulum-go
```

## Install to PATH

```bash
make install
```

Default prefix is `/usr/local`. Override with `make install PREFIX=/opt/reticulum`.

Or install into your Go binary directory:

```bash
CGO_ENABLED=0 go install -ldflags="-s -w" ./cmd/reticulum-go
```

## First run

```bash
make run
```

Or:

```bash
go run ./cmd/reticulum-go
```

On first start the daemon creates `~/.reticulum-go/` with a default config if none exists. Logs go to stderr. Set verbosity with `[logging] loglevel` in the config file (0 through 7, same scale as Python).

### Custom config path

Pass `--config /path/to/config` to use a Python-style location such as `/etc/reticulum/config` or `~/.reticulum/config`.

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

UDP requires an explicit `target_address` or `target_host`. Open binds do not learn peers from the first inbound packet (same policy as Python `forward_ip`).

For local mesh discovery over IPv6 link-local multicast, use `AutoInterface`. See [Interfaces](interfaces.md).

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

## Enable the control API

Add to `[reticulum]`:

```ini
enable_control_api = yes
rpc_key = <64 hex characters>
control_api_host = 127.0.0.1
control_api_port = 37430
```

Generate a random 32-byte key and encode as hex. Clients send `Authorization: Bearer <rpc_key>`. See [Control API](control-api.md).

## Disable the sandbox

Sandboxing is on by default. To turn it off (not recommended for production):

```ini
enable_sandbox = no
```

See [Security](security.md) for platform behavior.

## Troubleshooting

**Daemon exits on config error.** Check the config path and syntax. Unknown keys are ignored so a damaged file can still boot. Fix typos in `type` and interface names.

**No paths to remote destinations.** Confirm interfaces are enabled, peers are reachable, and transport is enabled. Use debug level 5 or higher temporarily. Request paths explicitly from application code or the control API.

**IFAC mismatches.** Peers must use the same `network_name` and `passphrase`. Wrong IFAC frames are dropped silently on ingress.

**Shared instance conflicts.** Only one process should own interfaces when `share_instance = yes`. Others should connect as clients. Check `shared_instance_port` (default 37428).

**Permission errors on Linux sandbox.** Landlock requires kernel 5.13+. The config directory and storage paths must live under whitelisted locations. See [Security](security.md).

## Next steps

| Goal | Document |
|------|----------|
| Configure interfaces and rates | [Configuration](configuration.md), [Interfaces](interfaces.md) |
| Write a Go app | [Package map](package-map.md), [Embedding and WebAssembly](embedding-and-wasm.md) |
| Use Python interop | [Compatibility](compatibility.md) |
| Run examples | [Examples](examples.md) |
