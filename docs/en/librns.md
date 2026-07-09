# librns C ABI

| Field | Value |
|-------|-------|
| Document version | 1.0 |
| Last updated | 2026-07-09 |
| Author | Ivan |
| Header version | `RNS_API_VERSION` 1.0 |

## Purpose

`librns` embeds Reticulum in-process for native hosts (C, C++, Qt, Flutter FFI, and similar). It is a thin facade over `pkg/node`, destination, and link. Same wire stack as the daemon. Not a Python API and not a full Control API mirror.

For Go apps, prefer `pkg/node` directly. For a separate daemon and JSON/WebSocket, use the [Control API](control-api.md).

## Artifacts

| Artifact | Role |
|----------|------|
| `include/rns.h` | Public C header |
| `bin/librns.so` | Shared library (Linux first) |
| `pkg/librns` | Pure Go facade (tests and fuzz without CGO) |
| `pkg/librns/capi` | CGO `//export` shims |
| `cmd/librns` | `-buildmode=c-shared` entry |
| `examples/librns-smoke` | Minimal C smoke program |

Daemon builds stay `CGO_ENABLED=0`. Only `build-librns` turns CGO on.

## Build and smoke

```bash
task build-librns
make -C examples/librns-smoke
./examples/librns-smoke/librns-smoke
```

Needs a C toolchain and CGO. Output: `bin/librns.so` and a copy of the header under `bin/rns.h`.

## librns vs Control API

| librns | Control API |
|--------|-------------|
| In-process | Separate `reticulum-go` process |
| `rns_event_poll` queue | WebSocket events |
| C ABI / FFI | JSON over HTTP and WS |
| Caller-owned buffers | Base64 / hex JSON payloads |
| Minimal node and link surface | Sessions, requests, lifecycle HTTP |

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
| `RNS_ERR_NOT_FOUND` | e.g. unknown destination identity for link open |
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
| `rns_node_destroy` | Stops if needed, invalidates handle |
| `rns_node_set_identity` | Attach identity before destinations that need one |

### Identity

| Function | Notes |
|----------|-------|
| `rns_identity_generate` | New software identity |
| `rns_identity_load` | Path from operator config. Rejects empty and NUL |
| `rns_identity_destroy` | Release handle |
| `rns_identity_hash` | Truncated hash as 32 hex chars |

### Destination

| Function | Notes |
|----------|-------|
| `rns_destination_create` | App name required. Optional aspects. `accepts_links` wires inbound links |
| `rns_destination_announce` | Optional app data |
| `rns_destination_hash` | 16-byte truncated hash (`RNS_HASH_LEN`) |
| `rns_destination_destroy` | Release handle |

### Path and link

| Function | Notes |
|----------|-------|
| `rns_path_request` | Requires started node and 16-byte dest hash |
| `rns_link_open` | Outbound link to dest hash (identity must be known from announce) |
| `rns_link_send` | On established link |
| `rns_link_close` | Teardown |
| `rns_link_id` | 16-byte link id |

### Events

| Function | Notes |
|----------|-------|
| `rns_event_poll` | Blocks up to `timeout_ms`. Returns `RNS_ERR_TIMEOUT` if empty |

| Kind | Meaning |
|------|---------|
| `RNS_EV_ANNOUNCE` | Announce received |
| `RNS_EV_LINK_ESTABLISHED` | Link up (inbound or outbound) |
| `RNS_EV_LINK_FAILED` | Open failed or timed out |
| `RNS_EV_LINK_DATA` | Payload on link |
| `RNS_EV_LINK_CLOSED` | Link torn down |

`rns_event` fields are filled by copy. Set `app_data` and `app_data_cap` before poll for variable payloads. Truncation sets `app_data_truncated` (and `error_message_truncated` for long errors).

The per-node queue is bounded. On overflow it drops the oldest event.

## ABI rules

- Handles are opaque `uint64_t`. Destroy them before process exit.
- Never hold Go pointers across the ABI. The facade always copies.
- Paths are operator-chosen. Empty and embedded NUL are rejected.
- Empty config path is valid and means in-memory defaults (no shared-instance bind).

## Not in this ABI (yet)

- Request / response bridge
- Path table introspection
- Lifecycle pause / resume (use `pkg/node` or Control API)
- Event callbacks (poll first. Optional callback later)
- macOS / Windows shared libs, Android NDK, Flutter package

Grow the header only when a real host needs it. Keep `RNS_API_VERSION` in mind.

## Typical flow

```
rns_node_create("")
rns_identity_generate / rns_identity_load
rns_node_set_identity
rns_node_start
rns_destination_create(..., accepts_links=1)
rns_destination_announce
  peer: rns_event_poll -> RNS_EV_ANNOUNCE
  peer: rns_link_open(dest_hash)
rns_event_poll -> RNS_EV_LINK_ESTABLISHED
rns_link_send / RNS_EV_LINK_DATA
rns_link_close / RNS_EV_LINK_CLOSED
rns_node_stop
rns_node_destroy
```

## Testing

| Kind | Where |
|------|-------|
| Unit and edge | `pkg/librns/*_test.go` |
| Facade link integration | `TestFacadeLinkOpenSendClose` |
| Property | `testing/quick` drop-oldest and handle table |
| Fuzz | `FuzzHandleTable`, `FuzzEventQueue`, `FuzzConfigPathCreate`, `FuzzValidatePath` |
| C smoke | `examples/librns-smoke` |

```bash
go test ./pkg/librns
task build-librns
make -C examples/librns-smoke && ./examples/librns-smoke/librns-smoke
```

## Related documents

- [Embedding and WebAssembly](embedding-and-wasm.md)
- [Control API](control-api.md)
- [Package map](package-map.md)
- [Compatibility](compatibility.md)
- [Examples](examples.md)
