# Examples

| Field | Value |
|-------|-------|
| Document version | 1.1 |
| Last updated | 2026-07-09 |
| Author | Ivan |

Sample programs under `examples/`. They show how to use Reticulum-Go as a library, beside the daemon, from C, or from the browser. They are starting points, not production services.

Pair this page with the [API reference](api-reference.md) for types and recipes.

## Which example to open

| Goal | Start here |
|------|------------|
| Smallest Go announce | `examples/minimal` |
| Announce utility | `examples/announce` |
| Request / echo service | `examples/echo` |
| Link establish and data | `examples/link` |
| Resource / file send | `examples/filetransfer` |
| Fetch Nomadnet-style pages | `examples/page-downloader` |
| Pages and files over Reticulum | `reticulum-go pageserver` or `examples/pageserver` |
| Browser WebSocket client | `examples/wasm` |
| Python talking to the daemon | `examples/control-client` |
| C / FFI smoke | `examples/librns-smoke` |
| Operator CLIs | `reticulum-go status|id|probe|path|cp` then [CLI utilities](utilities.md) |

## Library examples (Go)

Each directory has its own `go.mod`. Build inside the directory:

```bash
cd examples/minimal
go build -o minimal .
./minimal -config ~/.reticulum-go
```

| Directory | Demonstrates | Notes |
|-----------|--------------|-------|
| `examples/minimal` | Config, transport, destination, announce | Closest to Python `examples/minimal` |
| `examples/announce` | Announce utility patterns | |
| `examples/echo` | Request handler echo | |
| `examples/link` | Link establishment and payloads | |
| `examples/filetransfer` | Resource transfer | See local `README.md` |
| `examples/page-downloader` | Client fetch of `/page/` content | See `README.md` and `test.sh` |

Flags and ports differ per program. Prefer a dedicated config directory so examples do not fight a production daemon.

## pageserver

Preferred: `reticulum-go pageserver` (built into the main binary). Sample pages/files and a thin wrapper live under `examples/pageserver/`.

Serves content over Reticulum request handlers:

- `/page/` for HTML pages
- `/file/` for static files

Useful Nomadnet-style pattern. Live interop: `tests/interop/pageserver_live_test.go` with `RUN_LIVE_INTEROP=1`.

## wasm

Path: `examples/wasm/`

Browser chat demo using `pkg/wasm` (and Messenger helpers where configured).

```bash
task build-wasm
```

See [Embedding and WebAssembly](embedding-and-wasm.md).

## Control API client

Path: `examples/control-client/`

Python `client.py` for the localhost Control API: sessions, announces, WebSocket events, no Go in the application process.

Daemon config:

```ini
enable_control_api = yes
rpc_key = <64 hex characters>
```

See [Control API](control-api.md).

## librns smoke

Path: `examples/librns-smoke/`

Minimal C program against `librns.so` and `include/rns.h`.

```bash
task build-librns
make -C examples/librns-smoke
./examples/librns-smoke/librns-smoke
```

See [librns](librns.md).

## Mapping examples to API recipes

| Example | API reference section |
|---------|----------------------|
| `minimal` / `announce` | Quick start recipe, Destination announce |
| `echo` | Inbound link and request handler |
| `link` | Outbound link and request |
| `filetransfer` | Send a file resource |
| `control-client` | Other API surfaces → Control API |
| `librns-smoke` | Other API surfaces → librns |
| `wasm` | Other API surfaces → WASM |

## Interop and safety

Verify custom programs against `tests/interop/` with `RUN_LIVE_INTEROP=1` before relying on them on real meshes. Do not point example listeners at production identities without reviewing allow lists and link ACLs.

## Module layout

`examples/wasm` and `examples/pageserver` keep their own `go.mod` and `vendor/` trees. Build from those directories or via Task targets at the repository root. Smaller library examples also use per-directory modules so they can pin the local `reticulum-go` replace path.

## Related documents

- [API reference](api-reference.md)
- [Getting started](getting-started.md)
- [Embedding and WebAssembly](embedding-and-wasm.md)
- [Control API](control-api.md)
- [librns](librns.md)
- [CLI utilities](utilities.md)
