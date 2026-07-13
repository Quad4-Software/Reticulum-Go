# Examples

Sample programs under `examples/` demonstrate how to use Reticulum-Go as a library, beside the daemon, from C, or from the browser. They are starting points rather than production services.

Pair this page with the [API reference](api-reference.md) for types and recipes.

## Which Example to Open

| Goal | Start Here |
| :--- | :--- |
| Pages and files over Reticulum | `reticulum-go pageserver` or `examples/pageserver` |
| Browser WebSocket client | `examples/wasm` |
| Python talking to the daemon | `examples/control-client` |
| C / FFI integration smoke test | `examples/librns-smoke` |
| Operator CLIs | `reticulum-go status \| id \| probe \| path \| cp` then [CLI Utilities](utilities.md) |

## pageserver

Preferred: `reticulum-go pageserver` (built into the main binary). Sample pages, files, and a thin wrapper live under `examples/pageserver/`.

Serves content over Reticulum request handlers:

*   `/page/` for HTML pages
*   `/file/` for static files

This is a useful Nomadnet style pattern. Live interoperability is tested via `tests/interop/pageserver_live_test.go` when `RUN_LIVE_INTEROP=1` is set.

## wasm

Path: `examples/wasm/`

Browser chat demo using `pkg/wasm` and Messenger helpers where configured.

To build the WebAssembly target, run:

```bash
task build-wasm
```

See [Embedding and WebAssembly](embedding-and-wasm.md) for more details.

## Control API Client

Path: `examples/control-client/`

Python `client.py` for the localhost Control API. It handles sessions, announces, and WebSocket events with no Go in the application process.

Daemon configuration required:

```ini
enable_control_api = yes
rpc_key = <64 hex characters>
```

See [Control API](control-api.md).

## librns smoke

Path: `examples/librns-smoke/`

Minimal C program demonstrating integration against `librns.so` and `include/rns.h`.

To build and run:

```bash
task build-librns
make -C examples/librns-smoke
./examples/librns-smoke/librns-smoke
```

See [librns](librns.md).

## Module Layout

`examples/wasm` and `examples/pageserver` keep their own `go.mod` and `vendor/` trees. You can build from those directories or via Task targets at the repository root.

## Related Documents

*   [API Reference](api-reference.md)
*   [Getting Started](getting-started.md)
*   [Embedding and WebAssembly](embedding-and-wasm.md)
*   [Control API](control-api.md)
*   [librns](librns.md)
*   [CLI Utilities](utilities.md)
