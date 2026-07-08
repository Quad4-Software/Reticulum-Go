# Examples

| Field | Value |
|-------|-------|
| Document version | 1.0 |
| Last updated | 2026-07-07 |
| Author | Ivan |

## Overview

The `examples/` directory contains sample programs that show how to use Reticulum-Go as a library or alongside the daemon. They are starting points, not production services.

## Public examples

### wasm

Path: `examples/wasm/`

WebAssembly chat demo using `pkg/wasm` and the Messenger helper from `reticulum-go-mf` (sibling project).

Build:

```bash
task build-wasm
```

Serves a browser UI that connects through WebSocket to a Reticulum gateway.

See [Embedding and WebAssembly](embedding-and-wasm.md).

### pageserver

Path: `examples/pageserver/`

Serves pages and static files over Reticulum request handlers:

- `/page/` for HTML pages
- `/file/` for static file delivery

Useful pattern for Nomadnet-style content over Reticulum paths.

Live interop test: `tests/interop/pageserver_live_test.go` with `RUN_LIVE_INTEROP=1`.

## Library examples

These directories contain Go source and may ship prebuilt binaries in release artifacts. Build from source with `go build` in each directory.

| Directory | Demonstrates |
|-----------|--------------|
| `examples/minimal` | Minimal announce with `APP_NAME = example_utilities` |
| `examples/announce` | Announce utility |
| `examples/echo` | Echo service over Reticulum |
| `examples/link` | Link establishment and data |
| `examples/filetransfer` | Resource transfer |
| `examples/page-downloader` | Client that fetches pages |

Typical build:

```bash
cd examples/minimal
go build -o minimal .
```

Run with a valid Reticulum config path or shared instance attachment as required by each program.

## Control API client

Path: `examples/control-client/`

Python client (`client.py`) for the localhost control API. Demonstrates sessions, announces, and WebSocket events without Go code in the application.

Requires daemon with:

```ini
enable_control_api = yes
rpc_key = ...
```

See [Control API](control-api.md).

## Choosing an example

| Goal | Start here |
|------|------------|
| Smallest Go program | `examples/minimal` |
| Browser client | `examples/wasm` |
| HTTP-like pages over Reticulum | `examples/pageserver` |
| File send | `examples/filetransfer` |
| Python or other language | `examples/control-client` |
| Link API | `examples/link` |

## Running against Python peers

Configure compatible interfaces and IFAC on both sides. Use separate config directories for Go and Python on the same host.

Verify with interop tests in `tests/interop/` before relying on custom examples in production meshes.

## Module layout

`examples/wasm` and `examples/pageserver` have their own `go.mod` and `vendor/` trees. Build them from their directories or via Task targets at the repository root.

## Related documents

- [Getting started](getting-started.md)
- [Configuration](configuration.md)
- [Development and testing](development-and-testing.md)
