# Compatibility with Python Reticulum

## Reference target

Reticulum-Go is tested against **Python RNS 1.4.2**, plus the [official network API reference](https://reticulum.network/manual/reference.html).

Crossref vectors clone the reference from `rns://7649a50d84610232d1416b41d2896aff/reticulum/reticulum` via [rngit](https://reticulum.network/manual/git.html) (`tests/crossref/run_crossref.sh`). The GitHub mirror is not used for vectors.

The detailed matrix with config key tables lives in [COMPATIBILITY.md](../../COMPATIBILITY.md). This page summarizes the key differences and design choices.

## Component parity

| Component | Status | Notes |
|-----------|--------|-------|
| Crypto | Complete | Curve25519, AES-256-CBC, HMAC-SHA256, HKDF. Crossref verified |
| Identity | Complete | Optional RHB1 hardware descriptor. Python from_file is 64-byte software only. Known-peer ratchets persist under `storage/ratchets/` |
| Destination | Complete | SINGLE, GROUP, PLAIN, LINK. Identity ratchets opt-in via EnableRatchets. GROUP uses shared Token keys (`CreateKeys` / `LoadPrivateKey`) |
| Packet | Complete | Header types 1 and 2, all packet types and contexts |
| Transport | Complete | Path table, announces, relay, persistence, ingress control, random-blob path selection |
| IFAC | Complete | UDP, TCP, Auto. Live tests in `tests/interop/ifac_live_test.go` |
| Link | Complete | Both directions, RTT, request/response, channel, buffer, resources |
| Resource | Complete | Multi-part, hashmaps, RESOURCE_PRF, bzip2, split advertisements |
| Channel | Complete | Ghost-envelope fix, RX sequence ring, TX window and MDU gates. Tests in `pkg/channel` |
| Buffer | Complete | Stream buffer over channel |
| Interfaces | Partial | See below |
| Discovery | Partial | rnstransport listening works. Announcer and autoconnect not auto-started |
| Blackhole | Partial | Local table, announce drop, and LINKIDENTIFY teardown. No publish/federation or `/list` destination |
| Node lifecycle | Go-only | `pkg/node` embedder API, no Python equivalent |
| librns C ABI | Go-only | `pkg/librns`, `include/rns.h`. See [librns](librns.md) |
| Odin librns bindings | Go-only host | `bindings/odin` (Linux). See [librns](librns.md#odin-bindings) |
| Dart librns FFI | Go-only host | `bindings/dart`. Linux, Android, Windows. See [librns](librns.md#dart-ffi-bindings) |
| Dart Control API client | Go-only host | `bindings/dart`. See [Control API](control-api.md#dart-and-flutter) |

## Interfaces

| Python | Reticulum-Go |
|--------|--------------|
| UDPInterface | Yes |
| TCPClientInterface, TCPServerInterface | Yes |
| AutoInterface | Yes (includes 1.3.5 roam listener swap) |
| I2PInterface | Yes |
| BackboneInterface | Yes |
| RNode, KISS, AX25KISS, Weave | No |
| SerialInterface | Yes |
| Modem73Interface | Yes |
| SDRInterface | Yes (lab/testing Go-native burst modem, mock/rtltcp, optional tagged USB backends) |
| PipeInterface | Yes |
| LocalInterface | Yes via share_instance and config LocalInterface / LocalServerInterface |
| External plugins | Yes (Go-native factories, manifests, executables under `interfaces/`) |
| WebSocket | Go-only |
| QUIC | Go-only (QUICClientInterface / QUICServerInterface) |
| WebTransport | Go-only (WebTransportClientInterface / WebTransportServerInterface) |
| DNSRendezvous | Go-only (DNSRendezvousInterface) |
| VSOCK | Go-only Linux (VSOCKClientInterface / VSOCKServerInterface) |
| HTTPS | Go-only (HTTPSClientInterface / HTTPSServerInterface) |

UDP requires explicit target_host or target_address (Python forward_ip policy).

Opt-in UDP reconnect when `max_reconnect_tries > 0` is a Go extension.

## Python 1.2.x to 1.4.2 changes

Wire format is stable across 1.2.x to 1.4.x. Notable behavior differences:

| Python change | Version | Go |
|---------------|---------|-----|
| BZ2 bomb limits | 1.1.9 | Covered |
| Path-request ingress/egress control | 1.2.5 | Covered |
| Path table random-blob selection | 1.2.x+ | Covered |
| Announce dedup when dest in path table | 1.3.4 | Covered |
| Blackhole link teardown at LINKIDENTIFY | 1.3.2 | Covered |
| AutoInterface roam listener replacement | 1.3.5 | Covered |
| Channel ghost envelopes | 1.3.0 | Covered |
| Channel RX sequence ring and send window/MDU | Python Channel | Covered |
| Shared-instance RPC msgpack | 1.3.4 | Covered |
| `MODE_INTERNAL` / recursive_prs / announce mode rules | 1.3.6 | Covered |
| Ephemeral transport identity / static_transport_identity | 1.3.6 | Covered |
| local_hops_delta hop mangling | 1.3.6 / 1.3.7 | Covered |
| Reject unpack when hops `>= PATHFINDER_M` | 1.3.8 | Covered |
| Link expected_hops on both sides / LRPROOF hop gate | 1.3.8 | Covered |
| Discovery on `MODE_INTERNAL` / location_cmd | 1.3.9 | Covered |
| LINKIDENTIFY once / receiver `RESOURCE_RCL` | 1.3.9 | Covered |
| Resource ADV size / empty HMU / Backbone flap block | 1.3.9 | Covered |
| Default discovery stamp 16 / stamp caches | 1.4.0 | Covered |
| Link keepalive when remote continuously transmits | 1.4.0 | Covered (`lastKeepaliveNs`) |
| Backbone blocked_ips / blocked_ip_list in ifstats | 1.4.0 | Covered |
| Known-destination background cleaning | 1.4.0 | Covered (Go path/age rules) |
| Interface gravity contests | 1.4.1 | Covered (Go pathingAffinity unique) |
| announces_to_internal / boundary search modes | 1.4.1 | Covered |
| LRPROOF path rebalance | 1.4.1 | Covered (Go dampening + gravity sticky) |
| max_request_size / max_response_size | 1.4.1 | Covered |
| Recursive PR online gate | 1.4.2 | Covered (Go also bitrate-gates + emit re-check) |
| Discovery blackhole filtering | 1.4.2 | Covered (mutation-invalidated set, receive-time drop) |

## Known gaps

| Gap | Impact |
|-----|--------|
| Discovery autoconnect | Listen, validate, and InterfaceAnnouncer work. Autoconnect loops are not auto-started |
| Blackhole federation | publish_blackhole, blackhole_sources, and updater not driven |
| Remote management mutate | Remote `/path` table and rates plus `/status` work. Remote drop, path-request, and blackhole mutate are unimplemented (Python also exits 255) |
| RNode and radio serial drivers | RNode / KISS / AX25 / Weave not in this tree. SerialInterface, Modem73Interface, and SDRInterface are present |
| Utilities rnir rnpkg rngit | Not ported |
| Python-only utilities | rnir, rnpkg, rngit, rnodeconf are not ported |

## Go-only extensions

These do not change the wire format:

| Feature | Package / location |
|---------|-------------------|
| Interface hot reload | `pkg/node/reload.go`, SIGHUP |
| watch_interfaces NIC polling | `pkg/node/netmon.go` |
| Node lifecycle API | OnNetworkAvailable, OnNetworkLost, RefreshPaths |
| UDP reconnect (opt-in) | `pkg/interfaces/reconnect.go` |
| Backbone I/O multiplexing | `pkg/backbone` |
| WebSocket interface | `pkg/interfaces/websocket_*.go` |
| QUIC interface | `pkg/interfaces/quic.go`, `quic_tls.go` |
| Control API | `pkg/controlapi` |
| librns C ABI | `pkg/librns`, `include/rns.h` ([librns](librns.md)) |
| Odin librns bindings | `bindings/odin` ([librns](librns.md#odin-bindings)) |
| Zig librns bindings | `bindings/zig` ([librns](librns.md#zig-bindings)) |
| C++ librns bindings | `bindings/cpp` ([librns](librns.md#c-bindings)) |
| Dart librns FFI | `bindings/dart` ([librns](librns.md#dart-ffi-bindings)) |
| Dart Control API client | `bindings/dart` ([Control API](control-api.md#dart-and-flutter)) |
| Runtime sandbox | `pkg/sandbox` |
| Sandbox profiles and extra paths | `sandbox_profile`, `sandbox_extra_paths`, `sandbox_strict`, `sandbox_exec_rlimits` |
| Control API Unix socket | `control_api_socket` in addition to TCP |
| Local mesh health counters | `pkg/health`, status RPC fields, `reticulum-go slow` findings |
| Pathing affinity / rebalance dampening | Gravity contests use live iface penalty. LRPROOF rebalance capped per dest and gravity-sticky |
| Local DoS protection | `pkg/protect`, config `dos_protection` (off/detect/prevent/auto), `storage/dos_protect.mpack` |
| RAM-only path tables | in_memory_path_table, in_memory_known_destinations |
| Fully ephemeral storage | in_memory_storage, `RETICULUM_IN_MEMORY_STORAGE`, soft caps |
| Stream read chunk | 64 KiB socket reads on TCP/QUIC/VSOCK/WebTransport/I2P/Local/Pipe/backbone. Packet MTU unchanged |
| HandlePacket worker pool | Fixed workers and bounded queue (`max_packet_handlers`). Overflow always sheds |
| node_profile | `default`, `core_router`, `embedded`. Fills unset knobs only |
| Packet hashlist | Compact open-addressing generations. Rotate at `max/2`. Default 1M when transport is on |

## Config differences

| Topic | Python | Reticulum-Go |
|-------|--------|--------------|
| Default config dir | `~/.reticulum`, `/etc/reticulum` | `~/.reticulum-go` |
| Parser | configobj | Hand-rolled in `pkg/reticulumconfig` |
| Unknown keys | Errors | Ignored |
| Comments | `#` | `#` and `;` |
| Identity storage | Per-name blobs | Per-hash blobs (loads Python known dest files) |
| Log destination | stdout / file / callback | stderr, file, both, syslog, journald (and combinations) |

## Utilities

| Python | Reticulum-Go |
|--------|--------------|
| rnsd | `reticulum-go` daemon |
| rnstatus | rgostatus (same RPC and remote `-R` dest as Python, extra Go integrity fields) |
| (none) | rgoslow / `reticulum-go slow` (bottleneck and local health findings, Go-only) |
| (none) | rgozen / `reticulum-go zen` (static path and link footgun scan for Go and optional Python, Go-only) |
| `Examples/Speedtest.py` | rgospeed / `reticulum-go speedtest` (loopback link throughput smoke) |
| rnid | rgoid (`.rid`/`.rsg`/`.rsm`/`.rfe` compatible) |
| rnprobe | rgoprobe |
| rnpath | rgopath (same RPC and remote `-R` dest as Python) |
| rncp | rgocp (send/listen/fetch on `rncp.receive`) |
| rnir, rnodeconf, rnpkg, rngit | Not ported (deferred post-1.0). rnx is `reticulum-go x`. |
| rnsh | Python rnsh talks to a Go listener on dest app `rnsh`. `reticulum-go sh` auto-detects that dest. Native `rgosh` dest is Go-only. Listen `-b PERIOD` / `-A` / `-C` match Python rnsh. Unix PTY. Windows listener is pipes only. |
| WASM | `reticulum-wasm` (Go-only) |
| librns | `librns.so` + `rns.h` (Go-only, Linux first) |
| Odin bindings | `bindings/odin` (links `librns.so`, [librns](librns.md#odin-bindings)) |
| Dart client | `bindings/dart` (rns_control FFI and Control API) |

Setup for mixing Python and Go tools against either daemon is documented in [CLI utilities](utilities.md#mixed-python-and-go-tools).

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

To let rgostatus query Python rnsd, point `-config` at `~/.reticulum` and align rpc_key when set. On Linux both stacks default to Unix RPC when shared_instance_type is unset. Go tools also fall back to TCP when the type is unset. See [CLI utilities](utilities.md).

## Related documents

- [COMPATIBILITY.md](../../COMPATIBILITY.md) full tables
- [CLI utilities](utilities.md) for Go CLI tools and RPC setup
- [Interfaces](interfaces.md)
- [Cryptography](cryptography.md)
- [Development and testing](development-and-testing.md)
