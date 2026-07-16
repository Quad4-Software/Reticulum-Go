# Examples

Sample programs under `examples/` show how to use Reticulum-Go as a library.
They are starting points rather than production services.

Pair this page with the [API reference](api-reference.md).

## Which Example to Open

| Goal | Start Here |
| :--- | :--- |
| Smallest stack bring-up | `examples/minimal` |
| Announce callbacks | `examples/announce` |
| Encrypted link packets | `examples/link` |
| Minimal resource send | `examples/resources` |
| File list and download | `examples/filetransfer` |
| Prove-all echo | `examples/echo` |
| Page request client | `examples/page-downloader` |
| Pages and files over Reticulum | `reticulum-go pageserver` or `examples/pageserver` |
| Browser WebSocket client | `examples/wasm` |
| Python talking to the daemon | `examples/control-client` |
| C / FFI smoke test | `examples/librns-smoke` |
| Operator CLIs | `reticulum-go status \| id \| probe \| path \| cp` then [CLI Utilities](utilities.md) |

## minimal

Path: `examples/minimal/`

Starts transport, creates an identity and destination, and lets you announce.

```bash
cd examples/minimal
go run .
```

## announce

Path: `examples/announce/`

Registers announce handlers and prints arriving announces with app_data.

## link

Path: `examples/link/`

Client/server encrypted link. Server prints its destination hash. Client connects with `-destination` and exchanges text packets.

```bash
# terminal 1
go run . -server -listen-port 4242

# terminal 2
go run . -destination <hash> -listen-port 4243 -target-port 4242
```

## resources

Path: `examples/resources/`

Minimal link resource transfer. Server accepts one resource and prints it. Client sends `-payload` over `SendResource`.

```bash
# terminal 1
go run . -server -listen-port 4242

# terminal 2
go run . -destination <hash> -listen-port 4243 -target-port 4242 -payload "hello"
```

Payloads larger than about 1 MiB use split resource advertisements automatically.

## filetransfer

Path: `examples/filetransfer/`

Serves a directory over a link and lets a client list and download files as resources.

```bash
go run . -server -serve ./test_serve -listen-port 4242
```

## echo

Path: `examples/echo/`

Destination with prove-all. Client sends a packet and waits for a proof.

## page-downloader

Path: `examples/page-downloader/`

Requests `/page/` style content from a pageserver-compatible peer.

## pageserver

Preferred: `reticulum-go pageserver` (built into the main binary). Sample pages and files live under `examples/pageserver/`.

Serves:

* `/page/` for HTML pages
* `/file/` for static files

Live interoperability is tested via `tests/interop/pageserver_live_test.go` when `RUN_LIVE_INTEROP=1` is set.

## wasm

Path: `examples/wasm/`

Browser chat demo using `pkg/wasm`.

```bash
task build-wasm
```

See [Embedding and WebAssembly](embedding-and-wasm.md).

## Control API Client

Path: `examples/control-client/`

Python `client.py` for the localhost Control API.

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

## Module Layout

Most examples keep their own `go.mod` with a `replace` pointing at the repository root. `examples/wasm` and `examples/pageserver` also vendor dependencies.

## Related Documents

* [API Reference](api-reference.md)
* [Getting Started](getting-started.md)
* [Links, channels, and resources](links-channels-and-resources.md)
* [Embedding and WebAssembly](embedding-and-wasm.md)
* [Control API](control-api.md)
* [librns](librns.md)
* [CLI Utilities](utilities.md)
