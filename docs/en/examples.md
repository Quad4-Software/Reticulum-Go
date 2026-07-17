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
| C librns page fetch | `examples/librns-page-fetch` |
| Odin librns page fetch | `examples/odin-page-fetch` |
| Zig librns page fetch | `examples/zig-page-fetch` |
| C librns pageserver | `examples/librns-pageserver` |
| Odin librns pageserver | `examples/odin-pageserver` |
| Zig librns pageserver | `examples/zig-pageserver` |
| Odin librns bindings | `bindings/odin` |
| Zig librns bindings | `bindings/zig` |
| Dart librns FFI and Control API | `bindings/dart` |
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

## librns page fetch (C)

Path: `examples/librns-page-fetch/`

NomadNet / pageserver style page request over the C ABI. Opens a path, waits for an announce, establishes a link, and prints the `/page/...` response.

```bash
task build-librns
make -C examples/librns-page-fetch
./examples/librns-page-fetch/librns-page-fetch \
  -c /path/to/config \
  92798ea245a0afcfa559348e42d628c6:/page/index.mu
```

Add a TCP or Backbone hub from [directory.rns.recipes](https://directory.rns.recipes/) to the config.

## Odin page fetch

Path: `examples/odin-page-fetch/`

Same flow as the C page-fetch example, using the Odin wrappers in `bindings/odin`.

```bash
task build-librns
make -C examples/odin-page-fetch
./examples/odin-page-fetch/odin-page-fetch \
  -c /path/to/config \
  92798ea245a0afcfa559348e42d628c6:/page/index.mu
```

## librns pageserver (C)

Path: `examples/librns-pageserver/`

NomadNet-compatible `nomadnetwork.node` destination that serves `/page/index.mu` over librns request handlers.

```bash
task build-librns
make -C examples/librns-pageserver
./examples/librns-pageserver/librns-pageserver \
  -c /path/to/config
```

Prints `DEST_HASH=...` on startup. Fetch with the C, Odin, or Zig page-fetch example.

Run helpers (Go is the default demo pageserver):

```bash
task example:pageserver
make -C examples/pageserver run

task example:pageserver:c
task example:pageserver:odin
task example:pageserver:zig
```

## Odin pageserver

Path: `examples/odin-pageserver/`

Same pageserver flow using the Odin bindings.

```bash
task build-librns
make -C examples/odin-pageserver
./examples/odin-pageserver/odin-pageserver \
  -c /path/to/config
```

## Zig page fetch

Path: `examples/zig-page-fetch/`

Same flow as the C page-fetch example, using the Zig wrappers in `bindings/zig`.

```bash
task build-librns
make -C examples/zig-page-fetch
./examples/zig-page-fetch/zig-page-fetch \
  -c /path/to/config \
  92798ea245a0afcfa559348e42d628c6:/page/index.mu
```

## Zig pageserver

Path: `examples/zig-pageserver/`

Same pageserver flow using the Zig bindings.

```bash
task build-librns
make -C examples/zig-pageserver
./examples/zig-pageserver/zig-pageserver \
  -c /path/to/config
```

## Odin librns bindings

Path: `bindings/odin/`

Idiomatic Odin package over `librns.so`. Requires Odin on `PATH` and a built shared library.

```bash
task build-librns
task test-odin
```

Import with a collection rooted at `bindings/odin`:

```odin
import rns "rns:rns"
```

Wrapped surface includes node lifecycle, identity, destination, path table, link send and request, and event poll. See [librns](librns.md#odin-bindings).

## Zig librns bindings

Path: `bindings/zig/`

Idiomatic Zig package over `librns.so`. Requires Zig 0.16.0 or later on `PATH` and a built shared library.

```bash
task build-librns
task test-zig
```

Import as `@import("rns")` from a `build.zig` dependency on `bindings/zig`. See [librns](librns.md#zig-bindings).

## Dart bindings

Path: `bindings/dart/`

Package `rns_control` includes librns FFI (`ffi.dart`) and a Control API client.

```bash
task build-librns
task test-dart
task build-librns-targets -- linux android windows
```

```yaml
dependencies:
  rns_control:
    path: /path/to/Reticulum-Go/bindings/dart
```

See [librns Dart FFI](librns.md#dart-ffi-bindings) and [Control API](control-api.md#dart-and-flutter).

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
