# Compatibility with Python Reticulum

| Field | Value |
|-------|-------|
| Document version | 1.1 |
| Last updated | 2026-07-09 |
| Author | Ivan |

## Reference target

Reticulum-Go is tested against **Python RNS 1.3.5** and the [official network API reference](https://reticulum.network/manual/reference.html).

Crossref vectors clone the reference from `rns://7649a50d84610232d1416b41d2896aff/reticulum/reticulum` via [rngit](https://reticulum.network/manual/git.html) (`tests/crossref/run_crossref.sh`). The GitHub mirror is not used for vectors.

The detailed matrix with config key tables lives in [COMPATIBILITY.md](../../COMPATIBILITY.md). This page summarizes decisions architects need.

## Component parity

| Component | Status | Notes |
|-----------|--------|-------|
| Crypto | Complete | Curve25519, AES-256-CBC, HMAC-SHA256, HKDF. Crossref verified |
| Identity | Complete | Optional RHB1 hardware descriptor. Python `from_file` is 64-byte software only |
| Destination | Complete | SINGLE, GROUP, PLAIN, LINK |
| Packet | Complete | Header types 1 and 2, all packet types and contexts |
| Transport | Complete | Path table, announces, relay, persistence, ingress control, random-blob path selection |
| IFAC | Complete | UDP, TCP, Auto. Live tests in `tests/interop/ifac_live_test.go` |
| Link | Complete | Both directions, RTT, request/response, channel, buffer, resources |
| Resource | Complete | Multi-part, hashmaps, RESOURCE_PRF, bzip2 bomb limits |
| Channel | Complete | Simpler single-outlet model vs Python 1.3.0 ghost fix |
| Buffer | Complete | Stream buffer over channel |
| Interfaces | Partial | See below |
| Discovery | Partial | rnstransport listening works. Announcer and autoconnect not auto-started |
| Blackhole | Partial | Table and announce drop. Link teardown at LINKIDENTIFY not implemented |
| Node lifecycle | Go-only | `pkg/node` embedder API, no Python equivalent |
| librns C ABI | Go-only | `pkg/librns`, `include/rns.h`. See [librns](librns.md) |

## Interfaces

| Python | Reticulum-Go |
|--------|--------------|
| UDPInterface | Yes |
| TCPClientInterface, TCPServerInterface | Yes |
| AutoInterface | Yes (includes 1.3.5 roam listener swap) |
| I2PInterface | Yes |
| BackboneInterface | Yes |
| RNode, Serial, KISS, AX25KISS, Weave | No |
| PipeInterface | Yes |
| LocalInterface | Yes via `share_instance` and config `LocalInterface` / `LocalServerInterface` |
| WebSocket | Go-only |

UDP requires explicit `target_host` or `target_address` (Python `forward_ip` policy).

Opt-in UDP reconnect when `max_reconnect_tries > 0` is a Go extension.

## Python 1.2.x to 1.3.5 changes

Wire format is stable across 1.2.x to 1.3.x. Notable behavior differences:

| Python change | Version | Go |
|---------------|---------|-----|
| BZ2 bomb limits | 1.1.9 | Covered |
| Path-request ingress/egress control | 1.2.5 | Covered |
| Path table random-blob selection | 1.2.x+ | Covered |
| Announce dedup when dest in path table | 1.3.4 | Covered |
| Blackhole link teardown at LINKIDENTIFY | 1.3.2 | **Gap** |
| AutoInterface roam listener replacement | 1.3.5 | Covered |
| Channel ghost envelopes | 1.3.0 | Simpler Go model |
| Shared-instance RPC msgpack | 1.3.4 | Covered |

## Known gaps

| Gap | Impact |
|-----|--------|
| Blackhole at LINKIDENTIFY | Blackholed peers may still complete link setup. Announces are still dropped |
| Transport probes | `respond_to_probes` / `allow_probes` ignored |
| `publish_blackhole` and related keys | Not auto-published |
| RNode and radio serial drivers | Cannot speak to RNode hardware from this tree |
| Python CLI utilities | Yes (core) | `rgostatus`, `rgoid`, `rgoprobe` with Python format/RPC interop |

## Go-only extensions

These do not change the wire format:

| Feature | Package / location |
|---------|-------------------|
| Interface hot reload | `pkg/node/reload.go`, SIGHUP |
| `watch_interfaces` NIC polling | `pkg/node/netmon.go` |
| Node lifecycle API | `OnNetworkAvailable`, `OnNetworkLost`, `RefreshPaths` |
| UDP reconnect (opt-in) | `pkg/interfaces/reconnect.go` |
| Backbone I/O multiplexing | `pkg/backbone` |
| WebSocket interface | `pkg/interfaces/websocket_*.go` |
| Control API | `pkg/controlapi` |
| librns C ABI | `pkg/librns`, `include/rns.h` ([librns](librns.md)) |
| Runtime sandbox | `pkg/sandbox` |
| RAM-only path tables | `in_memory_path_table`, `in_memory_known_destinations` |

## Config differences

| Topic | Python | Reticulum-Go |
|-------|--------|--------------|
| Default config dir | `~/.reticulum`, `/etc/reticulum` | `~/.reticulum-go` |
| Parser | configobj | Hand-rolled in `pkg/reticulumconfig` |
| Unknown keys | Errors | Ignored |
| Comments | `#` | `#` and `;` |
| Identity storage | Per-name blobs | Per-hash blobs (loads Python known dest files) |
| Log destination | Configurable | stderr only |

## Utilities

| Python | Reticulum-Go |
|--------|--------------|
| rnsd | `reticulum-go` daemon |
| rnstatus | `rgostatus` (shared-instance RPC, announce/PR rates, JSON) |
| rnid | `rgoid` (`.rid`/`.rsg`/`.rsm`/`.rfe` compatible) |
| rnprobe | `rgoprobe` |
| rncp, rnir, rnpath, rnx, rnodeconf, rnpkg, rngit, rnsh | Not ported. Primitives in `pkg/` |
| WASM | `reticulum-wasm` (Go-only) |
| librns | `librns.so` + `rns.h` (Go-only, Linux first) |

Setup for Go tools against Python `rnsd` (TCP shared-instance RPC, `rpc_key`, `-config`) is documented in [CLI utilities](utilities.md).

## Verification workflow

```bash
# Unit and package tests
make test

# Crossref against Python vectors
./tests/crossref/run_crossref.sh all

# Live Go/Python side by side
RUN_LIVE_INTEROP=1 go test -v ./tests/interop/...
```

## Running alongside Python

Use separate config directories (`~/.reticulum-go` vs `~/.reticulum`). Point interfaces at the same peers with matching IFAC and ports. Shared instance ports must not conflict if both try to own the same interface.

To let `rgostatus` query Python `rnsd`, set `shared_instance_type = tcp` and matching `instance_control_port` / `rpc_key` in the Python config, then restart `rnsd`. See [CLI utilities](utilities.md).

## Related documents

- [COMPATIBILITY.md](../../COMPATIBILITY.md) full tables
- [CLI utilities](utilities.md) for Go CLI tools and RPC setup
- [Interfaces](interfaces.md)
- [Cryptography](cryptography.md)
- [Development and testing](development-and-testing.md)
