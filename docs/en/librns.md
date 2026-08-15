# librns C ABI

## Purpose

librns embeds Reticulum in-process for native hosts (C, C++, and similar FFI). It is a thin facade over `pkg/node`, destination, and link. Same wire stack as the daemon. Not a Python API and not a full Control API mirror.

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
| `bindings/c/examples/smoke` | Minimal C smoke program |
| `bindings/c/examples/page-fetch` | C NomadNet-style page fetch over librns |
| `bindings/odin/examples/page-fetch` | Odin NomadNet-style page fetch over librns |
| `bindings/zig/examples/page-fetch` | Zig NomadNet-style page fetch over librns |
| `bindings/c/examples/pageserver` | C NomadNet-style pageserver over librns |
| `bindings/odin/examples/pageserver` | Odin NomadNet-style pageserver over librns |
| `bindings/zig/examples/pageserver` | Zig NomadNet-style pageserver over librns |
| `bindings/cpp/examples/smoke` | Minimal C++ smoke program over librns |
| `bindings/cpp/examples/page-fetch` | C++ NomadNet-style page fetch over librns |
| `bindings/cpp/examples/pageserver` | C++ NomadNet-style pageserver over librns |
| `bindings/odin` | Idiomatic Odin bindings and tests over `librns.so` |
| `bindings/zig` | Idiomatic Zig bindings and tests over `librns.so` |
| `bindings/cpp` | Idiomatic C++17 bindings and tests over `librns.so` |
| `bindings/dart` | Dart FFI (`ffi.dart`) plus Control API client |
| `bindings/rust` | Idiomatic Rust bindings and tests over `librns.so` |
| `bindings/python` | ctypes Python bindings and tests over `librns.so` |
| `bindings/lua` | LuaJIT FFI bindings and tests over `librns.so` |
| `bindings/swift` | SwiftPM bindings and tests over `librns.so` |
| `bindings/java` | JNA Java bindings and tests over `librns.so` |
| `bindings/kotlin` | Kotlin facade over the Java JNA bindings |

Daemon builds stay `CGO_ENABLED=0`. Only `build-librns` turns CGO on.

## Build and smoke

```bash
task build-librns
make -C bindings/c/examples/smoke
./bindings/c/examples/smoke/librns-smoke
```

Page fetch against a live NomadNet or pageserver peer:

```bash
make -C bindings/c/examples/page-fetch
./bindings/c/examples/page-fetch/librns-page-fetch \
  -c /path/to/config \
  <dest_hash>:/page/index.mu

make -C bindings/odin/examples/page-fetch
./bindings/odin/examples/page-fetch/odin-page-fetch \
  -c /path/to/config \
  <dest_hash>:/page/index.mu

make -C bindings/zig/examples/page-fetch
./bindings/zig/examples/page-fetch/zig-page-fetch \
  -c /path/to/config \
  <dest_hash>:/page/index.mu

make -C bindings/cpp/examples/page-fetch
./bindings/cpp/examples/page-fetch/cpp-page-fetch \
  -c /path/to/config \
  <dest_hash>:/page/index.mu
```

Configs need an online TCP or Backbone hub from [directory.rns.recipes](https://directory.rns.recipes/). `config.example` only has AutoInterface.

Pageserver peers (announce `nomadnetwork.node` and serve `/page/index.mu`):

```bash
make -C bindings/c/examples/pageserver
./bindings/c/examples/pageserver/librns-pageserver \
  -c /path/to/config

make -C bindings/odin/examples/pageserver
./bindings/odin/examples/pageserver/odin-pageserver \
  -c /path/to/config

make -C bindings/zig/examples/pageserver
./bindings/zig/examples/pageserver/zig-pageserver \
  -c /path/to/config

make -C bindings/cpp/examples/pageserver
./bindings/cpp/examples/pageserver/cpp-pageserver \
  -c /path/to/config
```

Needs a C/C++ toolchain and CGO. Output: `bin/librns.so` and a copy of the header under `bin/rns.h`. The Odin examples also need odin on `PATH`. The Zig examples need zig on `PATH`. The C++ examples need a C++17 compiler.

## librns vs Control API

| librns | Control API |
|--------|-------------|
| In-process | Separate `reticulum-go` process |
| rns_event_poll or callback | WebSocket events |
| C ABI / FFI | JSON over HTTP and WS |
| Caller-owned buffers | Base64 / hex JSON payloads |
| Node, link, request, path table, lifecycle | Sessions and full HTTP surface |

## Supported surface

Authoritative names live in `include/rns.h`. Summary below.

### Version and errors

| Function | Notes |
|----------|-------|
| rns_version | Returns `RNS_API_VERSION` string |
| rns_last_error | Copies last failing call message into caller buffer |

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
| rns_node_create | Empty path uses in-memory defaults with share_instance off |
| rns_node_start / rns_node_stop | Idempotent |
| rns_node_destroy | Stops if needed, clears callback, invalidates handle |
| rns_node_set_identity | Attach identity before destinations that need one |
| rns_node_pause | Network lost (OnNetworkLost) |
| rns_node_resume | Network available (OnNetworkAvailable) |
| rns_node_refresh_paths | Refresh watched paths, or pass packed 16-byte hashes |

### Identity

| Function | Notes |
|----------|-------|
| rns_identity_generate | New software identity |
| rns_identity_load | Path from operator config. Rejects empty and NUL |
| rns_identity_save | Write identity to path (standard file layout) |
| rns_identity_destroy | Release handle |
| rns_identity_hash | Truncated hash as 32 hex chars |

### Destination

| Function | Notes |
|----------|-------|
| rns_destination_create | App name required. Optional aspects. accepts_links wires inbound links |
| rns_destination_announce | Optional app data |
| rns_destination_hash | 16-byte truncated hash (`RNS_HASH_LEN`) |
| rns_destination_enable_ratchets | Path required for disk persist. Empty path enables in-memory ratchets |
| rns_destination_enforce_ratchets | Opt-in reject of identity-key ciphertext |
| rns_destination_destroy | Release handle |
| rns_destination_register_request_handler | Bridge path to `RNS_EV_REQUEST_INCOMING` |

### Path and link

| Function | Notes |
|----------|-------|
| rns_path_request | Requires started node and 16-byte dest hash |
| rns_path_table | Snapshot into caller array. `max_hops < 0` means no filter |
| rns_link_open | Outbound link. Waits a bitrate-sized path window, then handshake. Identity must be known from announce |
| rns_link_send | On established link |
| rns_link_send_resource | Transfer bytes as a link resource (optional rncp name) |
| rns_link_close | Teardown |
| rns_link_id | 16-byte link id |
| rns_link_request | Outbound request. Completion via response or failed events |
| rns_request_respond | Answer a pending `RNS_EV_REQUEST_INCOMING` with raw bytes |
| rns_request_respond_file | NomadNet `/file/` response `[filename, content]` (auto resource when large) |

### Events

| Function | Notes |
|----------|-------|
| rns_event_poll | Blocks up to timeout_ms. Returns `RNS_ERR_TIMEOUT` if empty |
| rns_set_event_callback | Optional. Drains the same queue. Pass NULL to clear |

| Kind | Meaning |
|------|---------|
| `RNS_EV_ANNOUNCE` | Announce received |
| `RNS_EV_LINK_ESTABLISHED` | Link up (inbound or outbound) |
| `RNS_EV_LINK_FAILED` | Open failed or timed out |
| `RNS_EV_LINK_DATA` | Payload on link |
| `RNS_EV_LINK_CLOSED` | Link torn down |
| `RNS_EV_REQUEST_INCOMING` | Inbound request. Call rns_request_respond or rns_request_respond_file |
| `RNS_EV_REQUEST_RESPONSE` | Outbound request succeeded |
| `RNS_EV_REQUEST_FAILED` | Outbound request failed or timed out |
| `RNS_EV_RESOURCE_STARTED` | Inbound resource transfer started |
| `RNS_EV_RESOURCE_CONCLUDED` | Inbound resource assembled (path may hold rncp name) |

rns_event fields are filled by copy. Set app_data and app_data_cap before poll for variable payloads. Truncation sets app_data_truncated (and path_truncated / error_message_truncated when those strings do not fit).

The per-node queue is bounded. On overflow it drops the oldest event. Poll and callback share that queue. Prefer one consumer style at a time.

## ABI rules

- Handles are opaque uint64_t. Destroy them before process exit.
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

Embedders should call `rns_version()` and compare to `RNS_API_VERSION` from the header they compiled against. Current ABI is **1.5**.

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
| Facade link integration | TestFacadeLinkOpenSendClose |
| Lifecycle, path table, callback | `extended_test.go` |
| Property | `testing/quick` drop-oldest and handle table |
| Fuzz | FuzzHandleTable, FuzzEventQueue, FuzzConfigPathCreate, FuzzValidatePath |
| C smoke | `bindings/c/examples/smoke` |
| Odin bindings | `bindings/odin` (`task test-odin`) |
| Zig bindings | `bindings/zig` (`task test-zig`) |
| C++ bindings | `bindings/cpp` (`task test-cpp`) |
| Dart FFI | `bindings/dart` (`task test-dart`) |
| Rust bindings | `bindings/rust` (`task test-rust`) |
| Python bindings | `bindings/python` (`task test-python`) |
| Lua bindings | `bindings/lua` (`task test-lua`) |
| Swift bindings | `bindings/swift` (`task test-swift`) |
| Java bindings | `bindings/java` (`task test-java`) |
| Kotlin bindings | `bindings/kotlin` (`task test-kotlin`) |

```bash
go test ./pkg/librns
task build-librns
make -C bindings/c/examples/smoke && ./bindings/c/examples/smoke/librns-smoke
task test-odin
task test-zig
task test-cpp
task test-dart
task test-rust
task test-python
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
| Version and errors | version, last_error, error_string, Error |
| Node lifecycle | node_create, node_start, node_stop, node_destroy, node_pause, node_resume, node_set_identity, node_refresh_paths |
| Identity | identity_generate, identity_load, identity_save, identity_destroy, identity_hash, identity_hash_bytes, identity_public_key, identity_from_public_key, identity_sign, identity_verify |
| RSG / RSM | rsg_create, rsg_validate, rsg_sign_file, rsg_verify_file, rsm_verify |
| Destination | destination_create, destination_announce, destination_hash, destroy, request handler register |
| Path / interfaces | path_request, path_table, interfaces_list |
| Link and requests | link_open, link_send, link_close, link_id, link_request, request_respond |
| Events | event_poll, set_event_callback, Destination_Data, helpers for app data and hashes |
| Raw ABI | foreign `rns_*` procs in `bindings/odin/rns/foreign.odin` |

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
| Version and errors | version, lastError, errorString, Error |
| Node lifecycle | nodeCreate, nodeStart, nodeStop, nodeDestroy, nodePause, nodeResume, nodeSetIdentity, nodeRefreshPaths |
| Identity | identityGenerate, identityLoad, identitySave, identityDestroy, identityHash, identityHashBytes, identityPublicKey, identityFromPublicKey, identitySign, identityVerify |
| RSG / RSM | rsgCreate, rsgValidate, rsgSignFile, rsgVerifyFile, rsmVerify |
| Destination | destinationCreate, destinationAnnounce, destinationHash, destroy, request handler register |
| Path / interfaces | pathRequest, pathTable, interfacesList |
| Link and requests | linkOpen, linkSend, linkClose, linkId, linkRequest, requestRespond |
| Events | eventPoll, setEventCallback, destination_data, helpers for app data and hashes |
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

## C++ bindings

Path: `bindings/cpp/`.

Idiomatic C++17 RAII wrappers over the same C ABI (`include/rns.h`). Include the umbrella header and link `librns.so` (plus `bindings/cpp/src/event.cpp` for the event callback trampoline, or the rns_cpp CMake target):

```cpp
#include <rns/rns.hpp>

auto node_r = rns::Node::create(config_path);
if (!node_r.ok()) {
  return 1;
}
auto node = std::move(node_r).value();
```

### Coverage

| Area | C++ surface |
|------|-------------|
| Version and errors | version, last_error, error_string, Error, Result |
| Node lifecycle | `Node::create`, start, stop, pause, resume, set_identity, refresh_paths |
| Identity | `Identity::generate`, load, save, hash, hash_bytes, public_key, from_public_key, sign, verify |
| RSG / RSM | rsg_create, rsg_validate, rsg_sign_file, rsg_verify_file, rsm_verify |
| Destination | `Destination::create`, announce, hash, register_request_handler |
| Path / interfaces | path_request, path_table, interfaces_list |
| Link and requests | `Link::open`, send, close, id, request, request_respond |
| Events | `Node::poll`, set_event_callback, DestinationData, Event accessors |

Linux only (matches `librns.so`). Requires CMake and a C++17 compiler, plus a built shared library.

### Build and test

```bash
task build-librns
task test-cpp
# or
make -C bindings/cpp test
```

CI runs the same suite (`test-cpp` job).

Tests cover version and node lifecycle, identity and destination helpers, and a live UDP announce, link, and send round trip through `librns.so`.

## Dart FFI bindings

Path: `bindings/dart/` (import `package:rns_control/ffi.dart`).

In-process `dart:ffi` wrappers over the same C ABI. First platforms: Linux desktop, Android (`arm64-v8a`, `armeabi-v7a`, x86_64), and Windows amd64.

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

Android builds need an NDK (`ANDROID_NDK_HOME`). Windows cross-builds need mingw-w64 or Zig (`scripts/cc-windows-zig.sh`). Flutter apps copy Android ABIs into jniLibs and ship `librns.dll` beside the Windows runner.

For out-of-process Dart or Flutter without shipping native code, use the [Control API client](control-api.md#dart-and-flutter) in the same package.

## Rust bindings

Path: `bindings/rust/`.

Safe Rust wrappers over the same C ABI (`extern "C"` in `src/ffi.rs`). Link `librns.so` via `build.rs` / `RNS_LIB_DIR`.

```rust
use rns::{version, Identity, Node, API_VERSION};
assert_eq!(version(), API_VERSION);
```

### Coverage

| Area | Rust surface |
|------|--------------|
| Version and errors | version, last_error, Error, Result |
| Node lifecycle | `Node::create`, start, stop, set_identity, pause, resume, event_poll |
| Identity | generate, load, save, hash_hex, hash_bytes, public_key, from_public_key, sign, verify |
| RSG / RSM | rsg_create, rsg_validate, rsm_verify |
| Interfaces | interfaces_list |

```bash
task build-librns
task test-rust
# or
make -C bindings/rust test
```

## Python bindings

Path: `bindings/python/`.

ctypes wrappers over the same C ABI. Set `RNS_LIB_PATH` or place `bin/librns.so` on the default search path.

```python
import rns
assert rns.version() == rns.API_VERSION
```

### Coverage

| Area | Python surface |
|------|----------------|
| Version and errors | version, last_error, Error, map_code |
| Node lifecycle | `Node.create`, start, stop, set_identity, pause, resume, event_poll |
| Identity | generate, load, save, hash_hex, hash_bytes, public_key, from_public_key, sign, verify |
| RSG / RSM | rsg_create, rsg_validate, rsm_verify |
| Interfaces | interfaces_list |

```bash
task build-librns
task test-python
# or
make -C bindings/python test
```

## Lua bindings

Path: `bindings/lua/`.

LuaJIT FFI wrappers over the same C ABI. Requires LuaJIT (`ffi`). Set `RNS_LIB_PATH` or place `bin/librns.so` on the default search path.

```lua
local rns = require("rns")
assert(rns.version() == rns.API_VERSION)
```

```bash
task build-librns
task test-lua
# or
make -C bindings/lua test
```

## Swift bindings

Path: `bindings/swift/`.

SwiftPM library over the same C ABI (`Sources/CRNS` system module + idiomatic `RNS` target). Link `librns` via Makefile `-Xlinker` flags / `LD_LIBRARY_PATH`.

```swift
import RNS
assert(version() == API_VERSION)
```

```bash
task build-librns
task test-swift
# or
make -C bindings/swift test
```

## Java bindings

Path: `bindings/java/`.

JNA wrappers over the same C ABI (`io.quad4.rns`). Downloads `jna.jar` on first build. Set `RNS_LIB_PATH` or place `bin/librns.so` on the default search path.

```java
import io.quad4.rns.Rns;
assert Rns.version().equals(Rns.API_VERSION);
```

```bash
task build-librns
task test-java
# or
make -C bindings/java test
```

## Kotlin bindings

Path: `bindings/kotlin/`.

Kotlin facade over the Java JNA bindings (`io.quad4.rns.kotlin`). Requires `kotlinc` and the Java package.

```kotlin
import io.quad4.rns.kotlin.RnsKt
check(RnsKt.version() == RnsKt.API_VERSION)
```

```bash
task build-librns
task test-kotlin
# or
make -C bindings/kotlin test
```

## Related documents

- [Embedding and WebAssembly](embedding-and-wasm.md)
- [Control API](control-api.md)
- [Package map](package-map.md)
- [Compatibility](compatibility.md)
- [Examples](examples.md)
