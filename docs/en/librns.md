# librns C ABI

## Purpose

`librns` embeds Reticulum in-process for native hosts (C, C++, and similar FFI). It is a thin facade over `pkg/node`, destination, and link. Same wire stack as the daemon. Not a Python API and not a full Control API mirror.

For Go apps, prefer `pkg/node` directly. For a separate daemon and JSON/WebSocket, use the [Control API](control-api.md).

## Artifacts

| Artifact | Role |
|----------|------|
| `include/rns.h` | Public C header |
| `bin/librns.so` | Shared library (Linux) |
| `bin/darwin/*/librns.dylib` | Shared library (macOS) |
| `bin/windows/amd64/librns.dll` | Shared library (Windows) |
| `pkg/librns` | Pure Go facade (tests and fuzz without CGO) |
| `pkg/librns/capi` | CGO `//export` shims |
| `cmd/librns` | `-buildmode=c-shared` entry |
| `examples/librns-smoke` | Minimal C smoke program |
| `examples/librns-page-fetch` | C NomadNet-style page fetch over librns |
| `examples/odin-page-fetch` | Odin NomadNet-style page fetch over librns |
| `examples/zig-page-fetch` | Zig NomadNet-style page fetch over librns |
| `examples/librns-pageserver` | C NomadNet-style pageserver over librns |
| `examples/odin-pageserver` | Odin NomadNet-style pageserver over librns |
| `examples/zig-pageserver` | Zig NomadNet-style pageserver over librns |
| `bindings/odin` | Idiomatic Odin bindings and tests over `librns.so` |
| `bindings/zig` | Idiomatic Zig bindings and tests over `librns.so` |
| `bindings/dart` | Dart FFI (`ffi.dart`) plus Control API client |

Daemon builds stay `CGO_ENABLED=0`. Only `build-librns` turns CGO on.

## Build and smoke

```bash
task build-librns
make -C examples/librns-smoke
./examples/librns-smoke/librns-smoke
```

Page fetch against a live NomadNet or pageserver peer:

```bash
make -C examples/librns-page-fetch
./examples/librns-page-fetch/librns-page-fetch \
  -c /path/to/config \
  <dest_hash>:/page/index.mu

make -C examples/odin-page-fetch
./examples/odin-page-fetch/odin-page-fetch \
  -c /path/to/config \
  <dest_hash>:/page/index.mu

make -C examples/zig-page-fetch
./examples/zig-page-fetch/zig-page-fetch \
  -c /path/to/config \
  <dest_hash>:/page/index.mu
```

Configs need an online TCP or Backbone hub from [directory.rns.recipes](https://directory.rns.recipes/). `config.example` only has AutoInterface.

Pageserver peers (announce `nomadnetwork.node` and serve `/page/index.mu`):

```bash
make -C examples/librns-pageserver
./examples/librns-pageserver/librns-pageserver \
  -c /path/to/config

make -C examples/odin-pageserver
./examples/odin-pageserver/odin-pageserver \
  -c /path/to/config

make -C examples/zig-pageserver
./examples/zig-pageserver/zig-pageserver \
  -c /path/to/config
```

Needs a C toolchain and CGO. Output: `bin/librns.so` and a copy of the header under `bin/rns.h`. The Odin examples also need `odin` on `PATH`. The Zig examples need `zig` on `PATH`.

## librns vs Control API

| librns | Control API |
|--------|-------------|
| In-process | Separate `reticulum-go` process |
| `rns_event_poll` or callback | WebSocket events |
| C ABI / FFI | JSON over HTTP and WS |
| Caller-owned buffers | Base64 / hex JSON payloads |
| Node, link, request, path table, lifecycle | Sessions and full HTTP surface |

## Supported surface

Authoritative names live in `include/rns.h`. Summary below.

### Version and errors

| Function | Notes |
|----------|-------|
| `rns_version` | Returns `RNS_API_VERSION` string |
| `rns_last_error` | Copies last failing call message into caller buffer |

| Code | Meaning |
|------|---------|
| `RNS_OK` | Success |
| `RNS_ERR_INVALID_ARG` | Bad argument (empty path, wrong hash length, NUL in path) |
| `RNS_ERR_INVALID_HANDLE` | Unknown or destroyed handle |
| `RNS_ERR_NOT_FOUND` | Unknown destination identity or request id |
| `RNS_ERR_STATE` | Wrong lifecycle state (not started, no identity) |
| `RNS_ERR_IO` | Config or identity file I/O |
| `RNS_ERR_INTERNAL` | Unexpected internal failure |
| `RNS_ERR_TIMEOUT` | Event poll timed out |
| `RNS_ERR_TRUNCATED` | Output buffer too small |

### Node

| Function | Notes |
|----------|-------|
| `rns_node_create` | Empty path uses in-memory defaults with `share_instance` off |
| `rns_node_start` / `rns_node_stop` | Idempotent |
| `rns_node_destroy` | Stops if needed, clears callback, invalidates handle |
| `rns_node_set_identity` | Attach identity before destinations that need one |
| `rns_node_pause` | Network lost (`OnNetworkLost`) |
| `rns_node_resume` | Network available (`OnNetworkAvailable`) |
| `rns_node_refresh_paths` | Refresh watched paths, or pass packed 16-byte hashes |

### Identity

| Function | Notes |
|----------|-------|
| `rns_identity_generate` | New software identity |
| `rns_identity_load` | Path from operator config. Rejects empty and NUL |
| `rns_identity_save` | Write identity to path (standard file layout) |
| `rns_identity_destroy` | Release handle |
| `rns_identity_hash` | Truncated hash as 32 hex chars |

### Destination

| Function | Notes |
|----------|-------|
| `rns_destination_create` | App name required. Optional aspects. `accepts_links` wires inbound links |
| `rns_destination_announce` | Optional app data |
| `rns_destination_hash` | 16-byte truncated hash (`RNS_HASH_LEN`) |
| `rns_destination_destroy` | Release handle |
| `rns_destination_register_request_handler` | Bridge path to `RNS_EV_REQUEST_INCOMING` |

### Path and link

| Function | Notes |
|----------|-------|
| `rns_path_request` | Requires started node and 16-byte dest hash |
| `rns_path_table` | Snapshot into caller array. `max_hops < 0` means no filter |
| `rns_link_open` | Outbound link to dest hash (identity must be known from announce) |
| `rns_link_send` | On established link |
| `rns_link_send_resource` | Transfer bytes as a link resource (optional rncp `name`) |
| `rns_link_close` | Teardown |
| `rns_link_id` | 16-byte link id |
| `rns_link_request` | Outbound request. Completion via response or failed events |
| `rns_request_respond` | Answer a pending `RNS_EV_REQUEST_INCOMING` with raw bytes |
| `rns_request_respond_file` | NomadNet `/file/` response `[filename, content]` (auto resource when large) |

### Events

| Function | Notes |
|----------|-------|
| `rns_event_poll` | Blocks up to `timeout_ms`. Returns `RNS_ERR_TIMEOUT` if empty |
| `rns_set_event_callback` | Optional. Drains the same queue. Pass NULL to clear |

| Kind | Meaning |
|------|---------|
| `RNS_EV_ANNOUNCE` | Announce received |
| `RNS_EV_LINK_ESTABLISHED` | Link up (inbound or outbound) |
| `RNS_EV_LINK_FAILED` | Open failed or timed out |
| `RNS_EV_LINK_DATA` | Payload on link |
| `RNS_EV_LINK_CLOSED` | Link torn down |
| `RNS_EV_REQUEST_INCOMING` | Inbound request. Call `rns_request_respond` or `rns_request_respond_file` |
| `RNS_EV_REQUEST_RESPONSE` | Outbound request succeeded |
| `RNS_EV_REQUEST_FAILED` | Outbound request failed or timed out |
| `RNS_EV_RESOURCE_STARTED` | Inbound resource transfer started |
| `RNS_EV_RESOURCE_CONCLUDED` | Inbound resource assembled (`path` may hold rncp name) |

`rns_event` fields are filled by copy. Set `app_data` and `app_data_cap` before poll for variable payloads. Truncation sets `app_data_truncated` (and `path_truncated` / `error_message_truncated` when those strings do not fit).

The per-node queue is bounded. On overflow it drops the oldest event. Poll and callback share that queue. Prefer one consumer style at a time.

## ABI rules

- Handles are opaque `uint64_t`. Destroy them before process exit.
- Never hold Go pointers across the ABI. The facade always copies.
- Paths are operator-chosen. Empty and embedded NUL are rejected.
- Empty config path is valid and means in-memory defaults (no shared-instance bind).
- Incoming request handlers block the link goroutine until respond or a 30s timeout.

## Not in this ABI (yet)

- Full Control API surface (sessions, health JSON)

Grow the header only when a real host needs it. Keep `RNS_API_VERSION` in mind.

## Platform artifacts

Build with `task build-librns` for the host `.so`, or:

```bash
sh scripts/build-librns-targets.sh linux windows darwin android
```

| Platform | Output |
|----------|--------|
| Linux | `bin/librns.so` |
| Windows | `bin/windows/amd64/librns.dll` |
| macOS | `bin/darwin/amd64/librns.dylib` or `bin/darwin/arm64/librns.dylib` |
| Android | `bin/android/<abi>/librns.so` |

Embedders should call `rns_version()` and compare to `RNS_API_VERSION` from the header they compiled against. Current ABI is **1.3**.

## Typical flow

```
rns_node_create("")
rns_identity_generate / rns_identity_load
rns_node_set_identity
rns_node_start
rns_destination_create(..., accepts_links=1)
rns_destination_register_request_handler(dest, "/ping")
rns_destination_announce
  peer: rns_event_poll -> RNS_EV_ANNOUNCE
  peer: rns_link_open(dest_hash)
rns_event_poll -> RNS_EV_LINK_ESTABLISHED
rns_link_send / RNS_EV_LINK_DATA
rns_link_request / RNS_EV_REQUEST_RESPONSE
rns_link_close / RNS_EV_LINK_CLOSED
rns_node_stop
rns_node_destroy
```

## Testing

| Kind | Where |
|------|-------|
| Unit and edge | `pkg/librns/*_test.go` |
| Facade link integration | `TestFacadeLinkOpenSendClose` |
| Lifecycle, path table, callback | `extended_test.go` |
| Property | `testing/quick` drop-oldest and handle table |
| Fuzz | `FuzzHandleTable`, `FuzzEventQueue`, `FuzzConfigPathCreate`, `FuzzValidatePath` |
| C smoke | `examples/librns-smoke` |
| Odin bindings | `bindings/odin` (`task test-odin`) |
| Zig bindings | `bindings/zig` (`task test-zig`) |

```bash
go test ./pkg/librns
task build-librns
make -C examples/librns-smoke && ./examples/librns-smoke/librns-smoke
task test-odin
task test-zig
```

## Odin bindings

Path: `bindings/odin/`.

Idiomatic wrappers over the same C ABI (`foreign import system:rns`). Package import uses a collection rooted at `bindings/odin`:

```odin
import rns "rns:rns"
```

### Coverage

| Area | Odin surface |
|------|----------------|
| Version and errors | `version`, `last_error`, `error_string`, `Error` |
| Node lifecycle | `node_create`, `node_start`, `node_stop`, `node_destroy`, `node_pause`, `node_resume`, `node_set_identity`, `node_refresh_paths` |
| Identity | `identity_generate`, `identity_load`, `identity_save`, `identity_destroy`, `identity_hash` |
| Destination | `destination_create`, `destination_announce`, `destination_hash`, destroy, request handler register |
| Path | `path_request`, `path_table` |
| Link and requests | `link_open`, `link_send`, `link_close`, `link_id`, `link_request`, `request_respond` |
| Events | `event_poll`, `set_event_callback`, helpers for app data and hashes |
| Raw ABI | `foreign` `rns_*` procs in `bindings/odin/rns/foreign.odin` |

Linux only (matches `librns.so`). Requires Odin on `PATH` and a built shared library.

### Build and test

```bash
task build-librns
task test-odin
# or
make -C bindings/odin test
make -C bindings/odin smoke
```

CI runs the same suite (`test-odin` job, pinned Odin `dev-2026-06`).

Tests cover version and node lifecycle, identity and destination helpers, and a live UDP announce, link, and send round trip through `librns.so`.

## Zig bindings

Path: `bindings/zig/`.

Idiomatic wrappers over the same C ABI (`@extern` decls in `src/c.zig`). Import with a Zig package dependency on `bindings/zig`:

```zig
const rns = @import("rns");
```

### Coverage

| Area | Zig surface |
|------|-------------|
| Version and errors | `version`, `lastError`, `errorString`, `Error` |
| Node lifecycle | `nodeCreate`, `nodeStart`, `nodeStop`, `nodeDestroy`, `nodePause`, `nodeResume`, `nodeSetIdentity`, `nodeRefreshPaths` |
| Identity | `identityGenerate`, `identityLoad`, `identitySave`, `identityDestroy`, `identityHash` |
| Destination | `destinationCreate`, `destinationAnnounce`, `destinationHash`, destroy, request handler register |
| Path | `pathRequest`, `pathTable` |
| Link and requests | `linkOpen`, `linkSend`, `linkClose`, `linkId`, `linkRequest`, `requestRespond` |
| Events | `eventPoll`, `setEventCallback`, helpers for app data and hashes |
| Raw ABI | `c.rns_*` in `bindings/zig/src/c.zig` |

Linux only (matches `librns.so`). Requires Zig 0.16.0 or later on `PATH` and a built shared library.

### Build and test

```bash
task build-librns
task test-zig
# or
make -C bindings/zig test
```

CI runs the same suite (`test-zig` job, pinned Zig `0.16.0`).

Tests cover version and node lifecycle, identity and destination helpers, and a live UDP announce, link, and send round trip through `librns.so`.

## Dart FFI bindings

Path: `bindings/dart/` (import `package:rns_control/ffi.dart`).

In-process `dart:ffi` wrappers over the same C ABI. First platforms: Linux desktop, Android (`arm64-v8a`, `armeabi-v7a`, `x86_64`), and Windows amd64.

```dart
import 'package:rns_control/ffi.dart';

final rns = Rns();
final node = rns.nodeCreate();
rns.nodeStart(node);
```

### Artifacts

| Platform | Output |
|----------|--------|
| Linux | `bin/librns.so` |
| Android | `bin/android/<abi>/librns.so` |
| Windows | `bin/windows/amd64/librns.dll` |

```bash
task build-librns
task build-librns-targets -- linux android windows
task test-dart
```

Android builds need an NDK (`ANDROID_NDK_HOME`). Windows cross-builds need mingw-w64 or Zig (`scripts/cc-windows-zig.sh`). Flutter apps copy Android ABIs into `jniLibs` and ship `librns.dll` beside the Windows runner.

For out-of-process Dart or Flutter without shipping native code, use the [Control API client](control-api.md#dart-and-flutter) in the same package.

## Related documents

- [Embedding and WebAssembly](embedding-and-wasm.md)
- [Control API](control-api.md)
- [Package map](package-map.md)
- [Compatibility](compatibility.md)
- [Examples](examples.md)
