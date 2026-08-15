# Compatibility with Python Reticulum

This document compares Reticulum-Go with the official [Reticulum network API reference](https://reticulum.network/manual/reference.html) and the reference *rns* Python package (**RNS 1.4.2**).

Crossref tests clone the reference from `rns://7649a50d84610232d1416b41d2896aff/reticulum/reticulum` via [rngit](https://reticulum.network/manual/git.html) (`tests/crossref/run_crossref.sh`). The GitHub mirror at [markqvist/Reticulum](https://github.com/markqvist/Reticulum) is no longer used for vectors.

For crypto and storage see [docs/en/cryptography.md](docs/en/cryptography.md). For behavior and threat model see [SECURITY.md](SECURITY.md). Full English docs: [docs/en/](docs/en/README.md).

## Table

| Component | Reticulum-Go | Notes |
|-----------|:------------:|-------|
| Crypto | Yes | Curve25519 (X25519 + Ed25519), AES-256-CBC, HMAC-SHA256, HKDF. Checked against Python in crossref tests. [1] [2] |
| Identity | Yes | Key generation, recall, sign/verify, encrypt/decrypt, ratchets. Optional 72-byte hardware-bound descriptor (RHB1). On-wire Ed25519 public key matches RNS.Identity. Python Identity.from_file expects the 64-byte software layout only today. [3] [4] |
| Destination | Yes | SINGLE, GROUP, PLAIN, LINK. Announce and request handlers, links in and out. GROUP uses shared Token keys (`CreateKeys` / `LoadPrivateKey`). `SetMaxRequestSize` (1.4.1). [5] |
| Packet | Yes | Header types 1 and 2, all packet types and contexts. Byte-for-byte parity in crossref. [1] [6] |
| Transport | Yes | Core wire behavior matches Python 1.4.2: path table, announces, RequestPath, hops, next-hop, type-2 rewrap, link-table forwarding, persistence, ingress control, random-blob path selection (1.3.4 dedup), interface gravity contests (1.4.1), LRPROOF hop rebalancing (1.4.1), recursive PR online gating (1.4.2), interface mode announce rules including announces_to_internal and MODE_INTERNAL (1.3.6/1.4.1), ephemeral transport identity when transport is off (1.3.6). Probe responses via respond_to_probes / allow_probes, local_hops_delta hop mangling, and blackhole teardown at LINKIDENTIFY are implemented. Incoming links use HandleIncomingLinkRequest. Unpack rejects hop counts at or above PATHFINDER_M (1.3.8). Link keepalive/stale timing matches 1.4.0 including initiator lastKeepaliveNs throttling. Background known-destination cleaning uses path/age rules with cooperative yields. [7] [8] [9] |
| Interfaces | Partial | See Interfaces below. [10] |
| Discovery (RNS.Discovery, rnstransport) | Yes | Mirrors wire constants, LXStamper, msgpack layouts. Default discovery stamp cost is 16 (RNS 1.4.0). Valid/invalid announce stamp caches and single-flight validation match Python. Blackholed transport_id / announcer identity (network_id) filtered at receive time (1.4.2 list filtering, fail-closed). discover_interfaces or per-interface discoverable = yes starts rnstransport listening via StartInterfaceDiscovery. InterfaceAnnouncer publishes discoverable TCP/Backbone/I2P (and related) interfaces. Autoconnect and BlackholeUpdater loops are not auto-started. Build with BuildAppData, decode with ValidateAndDecode. Separate from AutoInterface multicast discovery. [11] [12] |
| Blackhole | Partial | Covers table semantics, msgpack, expiry, MergeRemote, EncodeForRequest. Announces from listed identities are dropped. Links from blackholed identities are torn down at LINKIDENTIFY. /list over rnstransport needs the RNS Request layer (not ported). publish_blackhole, blackhole_sources, blackhole_update_interval are ignored (deferred). [13] |
| IFAC | Yes | Matches salt, HKDF identity, mask/unmask. UDP, TCP, Auto apply IFAC. Unauthenticated frames dropped. [14] [15] |
| Link | Yes | Both directions, RTT, request/response (`RequestLimited` / max_response_size 1.4.1), channel, buffer, resources. WatchAndReconnect and EnableLinkAutoReconnect use Reestablish on closed links. [8] [12] |
| Resource | Yes | Multi-part transfer, hashmaps, RESOURCE_PRF, bzip2, split advertisements. BZ2 bomb limits match Python 1.1.9. [16] |
| Channel | Yes | In-link reliable channel. Ghost-envelope fix: sequence and tx-ring emplace only after a successful outlet send (Python 1.3.0). RX ring delivers in sequence order and drops duplicates. Send refuses a full TX window and packed envelopes larger than outlet MDU. Accepts both transport and link ACTIVE status values. [17] |
| Buffer | Yes | Stream buffer over channel. [18] |
| Node lifecycle | Yes (Go-only) | Embedder API: OnNetworkAvailable, OnNetworkLost, RefreshPaths, ReloadInterfaces, control API lifecycle routes. No Python equivalent. watch_interfaces polls NIC up/down and address changes on Linux, Android, Windows, macOS, and BSD (any CPU arch). Stub on WASM. OnNetworkLost cancels in-flight WatchAndReconnect loops. ReloadInterfaces equality covers MTU, bitrate, prefer_ipv6, announce-rate, ingress/egress control, mode, gravity, announces_to_internal, and outgoing. [12] [19] |
| librns C ABI | Yes (Go-only) | In-process C facade over node, destination, and link. Linux shared library first. Same wire stack as the daemon. Not a Python API. Build with task build-librns. [20] [21] |
| Odin librns bindings | Yes (Go-only host) | Idiomatic Odin wrappers over librns. Build/test with task test-odin. [22] [21] |
| Zig librns bindings | Yes (Go-only host) | Idiomatic Zig wrappers over librns. Build/test with task test-zig. [23] [21] |
| C++ librns bindings | Yes (Go-only host) | Idiomatic C++17 RAII wrappers over librns. Build/test with task test-cpp. [24] [21] |
| Dart librns FFI | Yes (Go-only host) | ffi.dart bindings. Linux, Android, Windows. Build with task build-librns-targets. [25] [21] |
| Dart Control API client | Yes (Go-only host) | Flutter-ready Dart client for the Control API (requests, resources, identify). Build/test with task test-dart. [25] [26] |

1. [tests/crossref](tests/crossref/)
2. [docs/en/cryptography.md](docs/en/cryptography.md)
3. [pkg/identity](pkg/identity/)
4. [RNS.Identity](https://github.com/markqvist/Reticulum/blob/master/RNS/Identity.py)
5. [pkg/destination](pkg/destination/)
6. [pkg/packet](pkg/packet/)
7. [pkg/transport](pkg/transport/)
8. [pkg/link](pkg/link/)
9. [pkg/identity/known_clean.go](pkg/identity/known_clean.go)
10. [Interfaces](#interfaces)
11. [pkg/discovery](pkg/discovery/)
12. [pkg/node](pkg/node/)
13. [pkg/blackhole](pkg/blackhole/)
14. [pkg/ifac](pkg/ifac/)
15. [tests/interop/ifac_live_test.go](tests/interop/ifac_live_test.go)
16. [pkg/resource](pkg/resource/)
17. [pkg/channel](pkg/channel/)
18. [pkg/buffer](pkg/buffer/)
19. [Node lifecycle](#node-lifecycle-go-only)
20. [pkg/librns](pkg/librns/), [include/rns.h](include/rns.h)
21. [docs/en/librns.md](docs/en/librns.md)
22. [bindings/odin](bindings/odin/)
23. [bindings/zig](bindings/zig/)
24. [bindings/cpp](bindings/cpp/)
25. [bindings/dart](bindings/dart/)
26. [docs/en/control-api.md](docs/en/control-api.md#dart-and-flutter)

## Interfaces

Python: [RNS/Interfaces](https://github.com/markqvist/Reticulum/tree/master/RNS/Interfaces). Go: [pkg/interfaces](pkg/interfaces/).

| Python Reticulum | Reticulum-Go | Where / notes |
|------------------|:------------:|---------------|
| UDPInterface | Yes | [udp.go](pkg/interfaces/udp.go). IFAC via [pkg/common](pkg/common/). Requires explicit target_host or target_address (Python forward_ip). Open binds do not learn peers from the first packet. Optional reconnect when `max_reconnect_tries > 0` (Go extension). |
| TCPClientInterface | Yes | [tcp.go](pkg/interfaces/tcp.go), HDLC, keepalives. Reconnect in [reconnect.go](pkg/interfaces/reconnect.go). Re-synthesizes tunnels on reconnect (SetTunnelSynth / onConnected). |
| TCPServerInterface | Yes | Accept loop, HDLC, IFAC. |
| AutoInterface | Yes | [auto.go](pkg/interfaces/auto.go). IPv6 link-local multicast, peer aging. NIC rescan with watch_interfaces ([auto_rescan.go](pkg/interfaces/auto_rescan.go)). Roam listener swap ([auto_roam.go](pkg/interfaces/auto_roam.go), 1.3.5). |
| I2PInterface | Yes | [i2p.go](pkg/interfaces/i2p.go), SAM in [pkg/i2p](pkg/i2p/). Direct STREAM CONNECT for outbound peers, `i2cp.leaseSetEncType=6,4`. Live: `RUN_LIVE_I2P=1`. Go/Python interop: `tests/interop/i2p_live_test.go`. |
| BackboneInterface | Yes | [backbone.go](pkg/interfaces/backbone.go), [backbone_client.go](pkg/interfaces/backbone_client.go). Multiplexed I/O in [pkg/backbone](pkg/backbone/). Live interop: [tests/interop/backbone_live_test.go](tests/interop/backbone_live_test.go). |
| RNodeInterface | No | No RNode serial driver. |
| RNodeMultiInterface | No | Depends on RNode driver. |
| SerialInterface | Yes | [serial.go](pkg/interfaces/serial.go). HDLC framing. Go adds chunked reads, frame-idle drops, reconnect limits, flow-control options, IFAC, receive-only, injectable ports, and live stats. Live: [tests/interop/serial_live_test.go](tests/interop/serial_live_test.go). |
| Modem73Interface | Yes | [modem73.go](pkg/interfaces/modem73.go). KISS data TCP plus JSON control TCP for modem73. Live: [tests/interop/modem73_live_test.go](tests/interop/modem73_live_test.go). |
| SDRInterface | Yes | Lab/testing. [sdr.go](pkg/interfaces/sdr.go), [pkg/sdr](pkg/sdr/). Mock, rtltcp, tagged rtlsdr/hackrf. Go burst modem (not air-compatible with Modem73/RNode). Live TX is operator-regulated. Live: RUN_LIVE_SDR=1 [tests/interop/sdr_live_test.go](tests/interop/sdr_live_test.go). |
| KISSInterface | No | Not implemented. |
| AX25KISSInterface | No | Not implemented. |
| PipeInterface | Yes | [pipe.go](pkg/interfaces/pipe.go). Subprocess stdin/stdout with HDLC framing and respawn. |
| LocalInterface | Yes | [local.go](pkg/interfaces/local.go), [sharedinstance](pkg/sharedinstance/). Automatic via share_instance or explicit LocalInterface / LocalServerInterface config blocks. |
| WeaveInterface | No | Not implemented. |
| Android KISS / RNode / Serial | No | Android-only. |
| External / plugin interfaces | Yes (Go-native) | Unknown type values load from `{config_dir}/interfaces/`: registered factories (RegisterExternalFactory), JSON manifests (`{Type}.json`), or executable pipe drivers. Process-isolated via PipeInterface rather than exec of Python modules. |
| Interface base | Yes | [interface.go](pkg/interfaces/interface.go), [constants.go](pkg/interfaces/constants.go). |
| WebSocket | Go-only | websocket_native.go, websocket_wasm.go. |
| QUIC | Go-only | [quic.go](pkg/interfaces/quic.go), [quic_tls.go](pkg/interfaces/quic_tls.go). QUICClientInterface / QUICServerInterface. HDLC over one stream. Yggdrasil-style mesh TLS (ephemeral self-signed, skip-verify, optional peer_key SPKI pin). Not on WASM. Live: [tests/interop/quic_live_test.go](tests/interop/quic_live_test.go). |
| WebTransport | Go-only | [webtransport.go](pkg/interfaces/webtransport.go). WebTransportClientInterface / WebTransportServerInterface over HTTP/3. Datagram (default), stream (HDLC), or dual modes. App protocol rns. |
| DNSRendezvous | Go-only | [dns_rendezvous.go](pkg/interfaces/dns_rendezvous.go). Discovers UDP peers from DNS TXT (`rns=udp://host:port`). Rendezvous underlay, not a DNS tunnel. Live: [tests/interop/dns_rendezvous_live_test.go](tests/interop/dns_rendezvous_live_test.go). |
| VSOCK | Go-only (Linux) | [vsock.go](pkg/interfaces/vsock.go). VSOCKClientInterface / VSOCKServerInterface over `AF_VSOCK` with HDLC. Live Local CID: [tests/interop/vsock_live_test.go](tests/interop/vsock_live_test.go). |
| HTTPS | Go-only | [https.go](pkg/interfaces/https.go). HTTPSClientInterface / HTTPSServerInterface TLS long-poll packet underlay (`POST .../send`, `GET .../poll`). Live: [tests/interop/https_live_test.go](tests/interop/https_live_test.go). |

### Interface reconnect

| Aspect | Python *rns* | Reticulum-Go |
|--------|--------------|---------------|
| TCP / backbone / QUIC client reconnect | Yes for TCP/backbone (max_reconnect_tries, 5 s wait) | Yes via [reconnect.go](pkg/interfaces/reconnect.go) (QUIC client included) |
| I2P peer reconnect | Yes (15 s fixed wait) | Yes ([i2p.go](pkg/interfaces/i2p.go)) |
| UDP reconnect | No | Yes when `max_reconnect_tries > 0` (Go extension, opt-in) |
| Default when max_reconnect_tries omitted | Unlimited (None) | Unlimited (`-1` via NormalizeMaxReconnectTries) |
| After max tries exhausted | Interface teardown | Teardown (onExhausted calls `Stop()`). Idle retry only if allowIdleRetry is set (off by default). |
| Connectivity hooks | N/A | ConnectivityNotifier for embedders |

### Interface hot reload (Go only)

Python has no equivalent. Convenience for long-running Go daemons.

ReplaceInterface, UnregisterInterface, and SetReticulumConfig swap interfaces without corrupting paths or links. Unregister clears paths, discovery state, announce bookkeeping, relay rows, and link-table entries for the removed iface.

[cmd/reticulum-go](cmd/reticulum-go/) wires ReloadInterfaces, config equality, SIGHUP reload, and coordinates `Stop()` with reload. Equality in [reload.go](pkg/node/reload.go) covers type, addresses, I2P, IFAC, Auto ports, MTU, bitrate, prefer_ipv6, announce-rate, ingress/egress control, mode, recursive_prs, announces_from_internal, and outgoing.

Tests: `interface_lifecycle_test.go`, `interface_scrub_links_test.go`, `interface_stress_race_test.go`, `reload_e2e_test.go`.

## Node lifecycle (Go only)

Go-only embedder API in [pkg/node](pkg/node/) and the control API (`POST /v1/lifecycle/{resume,pause,refresh-paths}`). There is no Python equivalent. The API surface is complete for what Go ships. Platform limits are listed below.

Native hosts that cannot import Go can use [pkg/librns](pkg/librns/) (`include/rns.h`, `bin/librns.so`) for the same in-process stack. Odin hosts use [bindings/odin](bindings/odin/). See [docs/en/librns.md](docs/en/librns.md). Out-of-process clients use the control API instead.

| API | Behavior |
|-----|-----------|
| OnNetworkAvailable | Clears link pause, rescans Auto NICs, starts offline interfaces, re-registers transport, optionally re-establishes watched links |
| OnNetworkLost | Sets link pause and cancels in-flight reconnect loops. Default mode calls `Disable()` on interfaces. |
| RefreshPaths | Expires stale paths and requests fresh paths for watched destinations and explicit API args |
| ReloadInterfaces | Hot-reloads `[[Interface]]` blocks from config |

watch_interfaces polls NIC state every 10s and calls OnNetworkAvailable on link or address changes (all desktop/mobile OS targets except WASM). Also enables AutoInterface NIC rescan. Python discover_interfaces starts rnstransport discovery (StartInterfaceDiscovery).

## Python 1.2.x to 1.4.2 changes vs Go

Wire format is unchanged in 1.2.x to 1.4.x. Most churn is utilities and transport behavior.

| Python change | Version | Go status |
|---------------|---------|-----------|
| BZ2 decompression bomb limits | 1.1.9 | Covered |
| Path-request ingress/egress control | 1.2.5 | Covered (`pkg/rate`, `pkg/transport/ingress.go`) |
| Path table random-blob selection | 1.2.x+ | Covered (`pkg/transport/path_selection.go`) |
| Announce dedup when dest already in path table | 1.3.4 | Covered |
| Blackhole link teardown at LINKIDENTIFY | 1.3.2 | Covered |
| AutoInterface link-local listener replacement on roam | 1.3.5 | Covered (`pkg/interfaces/auto_roam.go`) |
| Channel outlet ghost envelopes | 1.3.0 | Covered (send-before-emplace, sequence rewind on failed transmit) |
| Shared-instance RPC msgpack | 1.3.4 | Covered (`pkg/sharedinstance/rpc.go`) |
| `MODE_INTERNAL`, recursive_prs, announces_from_internal | 1.3.6 | Covered (config, announce forward rules, path discovery gate) |
| static_transport_identity / ephemeral transport identity | 1.3.6 | Covered (RPC auth keeps persisted identity) |
| local_hops_delta hop mangling | 1.3.6 / 1.3.7 | Covered (random delta 2-7 on local-origin hop-0 packets) |
| Shared-instance hop-delta edge cases | 1.3.7 | Covered (delta skipped when connected to shared instance) |
| Reject unpack when hops `>= PATHFINDER_M` | 1.3.8 | Covered (`pkg/packet.Unpack`) |
| Link expected_hops on initiator and responder | 1.3.8 | Covered (`pkg/link`, LRPROOF hop gate) |
| Link traffic stats use ciphertext length | 1.3.8 | Covered (Go already accounts packed wire size on send) |
| Discovery allowed on `MODE_INTERNAL` | 1.3.9 | Covered (Go never auto-demotes discoverable modes) |
| location_cmd discovery geo executable | 1.3.9 | Covered (`pkg/discovery`) |
| LINKIDENTIFY sets remote identity only once | 1.3.9 | Covered (`pkg/link`) |
| Receiver cancel sends `RESOURCE_RCL` | 1.3.9 | Covered (`pkg/link` ICL handler / reject) |
| Resource ADV transfer size `> MAX_EFFICIENT*3` rejected | 1.3.9 | Covered (`pkg/resource` unpack) |
| Empty HMU cancels transfer | 1.3.9 | Covered (`pkg/link`) |
| Backbone fast-flapping client block | 1.3.9 | Covered (`pkg/interfaces` BackboneInterface) |
| HDLC drop frames `<= HEADER_MINSIZE` | 1.3.9 | Covered (TCP/Backbone HDLC decoders) |
| Default discovery stamp value 16 | 1.4.0 | Covered (`pkg/discovery` DefaultStampValue) |
| Discovery valid/invalid stamp caches and validation lock | 1.4.0 | Covered (`pkg/discovery` InterfaceDiscovery) |
| Initiator keepalive when outbound quiet while inbound busy | 1.4.0 | Covered (`pkg/link` watchdog, `lastKeepaliveNs`) |
| Responder keepalive reply throttle | 1.4.0 | Covered (`pkg/link` ContextKeepalive) |
| Backbone blocked_ips in ifstats | 1.4.0 | Covered (`blocked_ips` / `blocked_ip_list` in RPC, status, control API). Go lists only IPs past grace, not every flap tracker entry |
| Known-destination background cleaning | 1.4.0 | Covered with Go-native age/path rules (`pkg/identity` CleanKnownDestinations). Fresh never-used entries are not deleted early (Python quirk avoided) |
| Interface gravity / default_gravity / autoconnect gravity | 1.4.1 | Covered (`gravity`, `default_gravity`, RPC ifstats). Go contests use effective pathingAffinity (configured gravity minus live iface penalty) |
| announces_to_internal | 1.4.1 | Covered (boundary may feed internal when set) |
| Boundary unknown-path search to boundary+gateway | 1.4.1 | Covered (`BoundarySearchModes`) |
| LRPROOF dynamic path rebalance | 1.4.1 | Covered (transit RemainingHops + terminus expected_hops). Go adds per-dest dampening (max 8/min) and refuses hop-increasing rebalance onto much weaker-gravity ingress |
| Destination set_max_request_size | 1.4.1 | Covered (`Destination.SetMaxRequestSize`) |
| Request max_response_size | 1.4.1 | Covered (`Link.RequestLimited`) |
| allow_link_path_rebalance | 1.4.1 | Covered (default true, config `allow_link_path_rebalance`) |
| Recursive path-request online gate | 1.4.2 | Covered (`ifaceReadyForPathRequest`). Go also refuses non-positive bitrate when exposed and re-checks at emit time |
| Discovery blackhole set for list filtering | 1.4.2 | Covered (`ActiveIdentitySet` + receive-time filter). Go invalidates on mutation instead of a 60s TTL and drops at receive rather than list-only |
| rngit / rnid / rnsh utilities | 1.2.x+ | Not ported (no wire impact; rnsh security fix is Python-only) |

### RNS 1.3.6 through 1.4.2 notes

**1.3.6** adds interface `MODE_INTERNAL` (0x07), recursive_prs / announces_from_internal interface options, announce broadcast mode rules, static_transport_identity, local_hops_delta hop mangling, and ephemeral transport identity when transport is disabled (RPC key still derived from the persisted identity).

**1.3.7** tightens shared-instance hop-delta handling when both ends of a link are local clients (instance_local_link). No new wire types.

**1.3.8** rejects packets whose hop field is `>= PATHFINDER_M` (128) during unpack, records expected_hops on the link responder from the RTT packet, and keeps initiator LRPROOF acceptance gated on matching hop count (or unknown `PATHFINDER_M`). Also fixes link TX byte accounting to use ciphertext size.

**1.3.9** allows discoverable announces on `MODE_INTERNAL`, adds location_cmd for geo fields, sets link remote identity only once at LINKIDENTIFY, has receivers emit `RESOURCE_RCL` when cancelling an incoming resource, rejects oversized resource advertisements at unpack (`t > MAX_EFFICIENT_SIZE*3`), cancels transfers on empty HMU, blocks fast-flapping Backbone clients, and drops HDLC frames at or below `HEADER_MINSIZE`. Also includes a critical `rnsh` security fix (Python utility only).

**1.4.0** raises the default interface-discovery stamp cost from 14 to 16, adds valid and invalid discovery stamp caches with single-flight validation (CPU relief), and fixes link keepalive so the initiator still probes when the remote side is continuously transmitting (stale detection stays inbound-based). Initiator probe throttling uses `lastKeepalive` / `lastKeepaliveNs` so one-way local data traffic does not suppress probes. Responder keepalive replies are throttled to one per keepalive interval. Backbone ifstats expose `blocked_ips` and `blocked_ip_list`. Go also runs background known-destination cleaning with path/age rules and cooperative yields. Persistence stays dirty-flag plus TryLock (no Python-style on-disk recombination, no `packet_hashlist.raw`).

**1.4.1** adds interface gravity path contests, `announces_to_internal`, boundary-mode discovery filtered to boundary+gateway, LRPROOF hop rebalancing (`allow_link_path_rebalance`), and request size caps (`set_max_request_size` / `max_response_size`). Go matches the Python config keys and wire decisions, then keeps Go-unique pathingAffinity (live iface penalty on gravity contests), rebalance dampening, and gravity-sticky refusal of hop-increasing rebalances onto weaker ingress.

**1.4.2** skips offline interfaces when emitting recursive path requests, and caches blackholed identities for faster discovery list filtering on mobile/RPC. Go matches the online PR gate, also refuses non-positive bitrate when that metric is exposed, re-checks readiness at emit time, rebuilds the blackhole membership set on mutation instead of a fixed 60s TTL, and applies discovery blackhole filtering at receive time so blackholed peers never enter discovery caches.

## Security and integrity behavior

| Topic | Python | Reticulum-Go |
|-------|--------|---------------|
| IFAC unauthenticated drop | Yes | Yes |
| Local integrity / health counters | No | Yes (`pkg/health`, observe only) |
| Ingress/egress announce and PR rate limits | Yes (1.2.5) | Yes |
| BZ2 bomb limits on resource/buffer | Yes (1.1.9) | Yes |
| Reject hop counts `>= PATHFINDER_M` on unpack | Yes (1.3.8) | Yes (`pkg/packet.Unpack`) |
| LRPROOF hop count vs expected_hops | Yes | Yes (`pkg/link`) |
| Blackholed identity announces | Dropped | Dropped |
| Blackholed identity links | Torn down at LINKIDENTIFY (1.3.2) | Torn down at LINKIDENTIFY |
| UDP peer binding | Requires explicit forward_ip | Requires explicit target_host / target_address |
| HDLC frame size cap | Yes | Yes (maxHDLC in tcp.go) |

## Retained Go improvements

Intentional extensions beyond upstream *rns*:

| Area | Go behavior |
|------|--------------|
| Path table persistence | RAM-only when no config path. Explicit opt-in for disk. |
| Interface hot reload | ReloadInterfaces, transport scrub on unregister |
| Node lifecycle API | OnNetworkAvailable, OnNetworkLost, RefreshPaths, control API |
| watch_interfaces | Linux/Android/Windows/macOS/BSD NIC poll plus AutoInterface rescan |
| UDP reconnect | Opt-in via `max_reconnect_tries > 0` |
| Backbone I/O | epoll/kqueue/io_uring multiplexing |
| WebSocket interface | Browser/WASM transport |
| QUIC interface | QUICClientInterface / QUICServerInterface, mesh TLS + optional peer_key |
| WebTransport interface | WebTransportClientInterface / WebTransportServerInterface |
| DNS rendezvous | DNSRendezvousInterface TXT to UDP peer |
| VSOCK interface | Linux VSOCKClientInterface / VSOCKServerInterface |
| HTTPS long-poll | HTTPSClientInterface / HTTPSServerInterface |
| Seccomp sandbox | Linux Landlock plus seccomp-bpf denylist (enable_sandbox / enable_seccomp) |
| Local mesh health | `pkg/health` counters at drop sites, integrity fields on interface_stats / status CLI / control API, `reticulum-go slow` findings. Observe only. No wire change. |
| Pathing affinity | Effective gravity for announce contests subtracts a bounded live iface path-failure penalty. Configured `gravity` numbers stay Python-compatible. |
| LRPROOF rebalance dampening | Caps hop rewrites per destination (8/min) and refuses hop-increasing rebalance when current path affinity is much higher than proof ingress. |
| Path-request readiness | Online gate plus optional non-positive bitrate refusal and emit-time re-check (stricter than Python online-only). |
| Blackhole membership set | Mutation-invalidated `ActiveIdentitySet` instead of a 60s TTL cache. Discovery filters at receive time. |
| Stream read chunk | 64 KiB reads on stream underlays. HDLC frame MTU unchanged. |
| HandlePacket pool | Fixed workers and bounded queue (`max_packet_handlers`). |
| node_profile | `core_router` / `embedded` fill unset knobs only. |

## Utilities

| Python utility | Reticulum-Go | Notes |
|----------------|:------------:|-------|
| rnsd | Yes | [cmd/reticulum-go](cmd/reticulum-go/) daemon (default subcommand) |
| rncp | Yes | `reticulum-go cp` ([pkg/cli](pkg/cli/), symlink rgocp). Link resource send/listen/fetch, destination `rncp.receive` |
| rnid | Yes | `reticulum-go id` (symlink rgoid). `.rid`/`.rsg`/`.rsm`/`.rfe` interop with Python rnid |
| rnir | No | Python stub identity resolver. Not ported |
| rnpath | Yes | `reticulum-go path` (symlink rgopath). Local/shared-instance path table, drop, blackhole. Remote `-R` table and rates over `rnstransport.remote.management` |
| rnprobe | Yes | `reticulum-go probe` (symlink rgoprobe) |
| rnstatus | Yes | `reticulum-go status` (symlink rgostatus). Shared-instance RPC including announce/PR rates. Remote `-R` over `rnstransport.remote.management`. Go daemons also expose local integrity counters. TCP RPC setup: [docs/en/utilities.md](docs/en/utilities.md) |
| (Go-only) | Yes | `reticulum-go slow` (symlink rgoslow). Bottleneck and local health findings. See [docs/en/utilities.md](docs/en/utilities.md#rgoslow) |
| Speedtest.py | Yes | `reticulum-go speedtest` (symlink rgospeed). Loopback link throughput smoke / liveness floor. See [docs/en/utilities.md](docs/en/utilities.md#rgospeed) |
| rnx | Yes | `reticulum-go x` (symlinks rgox, rnx). Destination `rnx.execute`, request path command. JSON stdout, Python exit codes |
| rnodeconf | No | Depends on RNode driver |
| rnpkg | No | Not ported |
| rngit | No | Git-over-Reticulum. No wire impact. |
| rnsh | Interop | Python rnsh talks to dest app `rnsh` on either stack. `reticulum-go sh` auto-detects that dest. Native `rgosh` dest is Go-only. Unix PTY. Windows listener uses pipes. |
| WASM build | Go-only | [cmd/reticulum-wasm](cmd/reticulum-wasm/), [pkg/wasm](pkg/wasm/) |

## Deferred for post-1.0

| Item | Notes |
|------|-------|
| RNode / KISS / AX25 / Weave drivers | Hardware radio interface stack |
| Discovery autoconnect loops | InterfaceAnnouncer and listen/validate work. Autoconnect is not auto-started |
| Blackhole auto-publish / blackhole_sources | Federation loops |
| rnir / rnpkg / rngit | Missing Python utilities (rnsh interops with rgosh on dest app `rnsh`) |
| Remote rnpath drop / path-request / blackhole mutate | Remote table, rates, and rnstatus `/status` work. Remote drop and blackhole mutate are still unimplemented (Python also exits 255) |

## Examples

| Directory | Status | Notes |
|-----------|:------:|-------|
| minimal | Present | Start transport, create destination, announce |
| announce | Present | Announce callbacks and app_data |
| link | Present | Client/server encrypted link and packets |
| resources | Present | Minimal SendResource / conclude callback |
| filetransfer | Present | Directory listing and file resources over a link |
| echo | Present | Prove-all echo destination |
| page-downloader | Present | Request pages over Reticulum |
| pageserver | Present | `reticulum-go pageserver` sample tree |
| wasm | Present | `//go:build js,wasm`. JS bridge via pkg/wasm |
| control-client | Present | Python Control API client |
| librns-smoke | Present | C ABI smoke test |
| odin-rns | Present | Odin bindings tests over librns (`task test-odin`) |
| zig-rns | Present | Zig bindings tests over librns (`task test-zig`) |
| cpp-rns | Present | C++ bindings tests over librns (`task test-cpp`) |
| rns_control | Present | Dart FFI and Control API client tests (`task test-dart`) |

## Configuration

Python defaults from `RNS.Reticulum.__create_default_config` and [RNS/Reticulum.py](https://github.com/markqvist/Reticulum/blob/master/RNS/Reticulum.py). Go parses the same shape in [internal/config](internal/config/config.go) (canonical) and [pkg/config](pkg/config/) (legacy).

### Config file format

| Aspect | Python *rns* | Reticulum-Go |
|--------|--------------|---------------|
| Parser | configobj / ConfigObj | Hand-rolled scanner in internal/config |
| Top-level sections | [reticulum], [logging], [interfaces] | Same |
| Nested interface header | [[Interface Name]] (depth 2) | Same. Depth 2+ starts an interface. |
| Comments | Number sign (`#`) | `#` and `;` (including end-of-line after a space) |
| Booleans | Yes/No/True/False | yes/no/true/false/on/off/1/0 |
| Unknown keys / bad lines | ConfigObj errors | Ignored so a damaged file can boot |
| UTF-8 BOM | Tolerated | Stripped at read |
| Missing file | `__create_default_config__()` | CreateDefaultConfig in internal/config |

### Config directory search path

```
# Python (typical order)
/etc/reticulum/config
~/.config/reticulum/config
~/.reticulum/config

# Reticulum-Go default
~/.reticulum-go/config    # use --config for a Python-style path
```

### Storage layout under the config directory

| Subpath | Python *rns* | Reticulum-Go |
|---------|----------------|---------------|
| config | Main file | Same (under ~/.reticulum-go by default) |
| logfile | When destination is file/both | Same (default `{config_dir}/logfile/reticulum.log`) |
| storage/ | Container | Same |
| storage/identities/ | Per-name blobs | Per-hash blobs |
| storage/cache/ | General cache | Present |
| storage/cache/announces/ | Announce cache | Present |
| storage/resources/ | Resource scratch | Present |
| storage/blackhole | msgpack table | [pkg/blackhole](pkg/blackhole/) |
| storage/ratchets/ | Known-peer public keys as `{ratchet, received}` msgpack plus destination-private signed lists at `EnableRatchets` path | Same Python known-peer layout. Local private lists use the path the app passes (pageserver: `{destination_hash}`, same as Python LXMF). `EnableRatchetsInMemory` and `in_memory_storage` skip disk |
| storage/destination_table | Path snapshot | Yes (flat msgpack, Python-compatible layout) |
| storage/known_destinations | Known destinations | Yes (loads Python byte-keyed files) |
| storage/transport_identity | Transport identity | Present |
| interfaces/ | Python plugin modules | Go-native: factories, JSON manifests, executable pipe drivers under `{config_dir}/interfaces/` |

### [reticulum] keys

| Key | Python *rns* | Reticulum-Go | Notes |
|-----|:------------:|:------------:|-------|
| enable_transport | Yes | Yes | Wired to Transport |
| share_instance | Yes | Yes | [pkg/sharedinstance](pkg/sharedinstance/). Server binds first, else client. |
| shared_instance_port | Yes | Yes | TCP port for shared instance |
| instance_control_port | Yes | Yes | RPC when this process owns the instance |
| instance_name | Yes | Yes | Unix socket name when `shared_instance_type = unix` |
| shared_instance_type | Yes | Yes | tcp or unix (unset: unix on Linux, tcp elsewhere, matching Python) |
| backbone_io | No | Yes | Go-only. auto, epoll, kqueue, io_uring, or go. |
| rpc_key | No | Yes | Hex key for shared-instance RPC auth |
| enable_sandbox | No | Yes | OS sandbox after startup (Landlock + seccomp on Linux) |
| enable_seccomp | No | Yes | Linux seccomp-bpf denylist after Landlock (default yes with sandbox) |
| enable_control_api | No | Yes | Localhost JSON API ([pkg/controlapi](pkg/controlapi/)) |
| control_api_host | No | Yes | Control API bind address |
| control_api_port | No | Yes | Control API port |
| panic_on_interface_error | Yes | Yes | Honoured on interface errors |
| in_memory_path_table | No | Yes | RAM-only path table |
| in_memory_known_destinations | No | Yes | RAM-only known destinations |
| in_memory_storage | No | Yes | Fully ephemeral storage (identity, blackhole, resources, tables) |
| soft_memory_limit | No | Yes | Soft heap limit (K/M/G or bytes) |
| dos_protection | No | Yes | Go-only local IDS/IPS. off, detect, prevent, or auto (default). Adaptive baselines, msgpack store `dos_protect.mpack`, auto promote to prevent, relearn on network or drift change |
| max_in_memory_paths | No | Yes | Soft path-table cap under in_memory_storage |
| max_in_memory_known_destinations | No | Yes | Soft known-dest cap under in_memory_storage |
| max_in_memory_resource_bytes | No | Yes | Soft split-resource staging budget under in_memory_storage |
| discover_interfaces | Yes | Yes | Starts rnstransport listening (StartInterfaceDiscovery). Per-interface discoverable also starts listening and the InterfaceAnnouncer. |
| watch_interfaces | No | Yes | Go-only. Polls NIC changes via `net.Interfaces` (Linux, Android, Windows, macOS, BSD). WASM stub. Enables AutoInterface rescan. |
| static_transport_identity | Yes (1.3.6+) | Yes | Keep persisted transport identity when transport is off |
| local_hops_delta | Yes (1.3.6+) | Yes | Outbound hop mangling on local-origin packets |
| respond_to_probes / allow_probes | Yes | Yes | Registers `rnstransport.probe` with PROVE_ALL |
| publish_blackhole | Yes | No | Not auto-published |
| blackhole_sources | Yes | No | Ignored |
| blackhole_update_interval | Yes | No | Ignored (Python 1.3.2) |
| network_identity | Yes | Yes | Loads or creates identity for discovery encrypt/decrypt and rnstransport.network destinations |

### [logging] keys

| Key | Python *rns* | Reticulum-Go | Notes |
|-----|:------------:|:------------:|-------|
| loglevel | Yes | Yes | Same 0 to 7 scale |
| destination | Yes | Yes | Python: stdout / file / callback. Go also accepts syslog, journald, and combinations such as `syslog+stderr` |
| logfile | Yes | Yes | Used when destination includes file |

### [[Interface Name]] keys

Parsed in [pkg/reticulumconfig/config.go](pkg/reticulumconfig/config.go). Unlisted keys are ignored. IFAC options applied via ApplyIFACFromConfig.

| Key | Python *rns* | Reticulum-Go | Applies to (Go) |
|-----|:------------:|:------------:|-----------------|
| type | Yes | Yes | All |
| enabled / interface_enabled | Yes | Yes | All |
| mode / interface_mode | Yes | Yes | All (includes internal 0x07, RNS 1.3.6+) |
| recursive_prs | Yes (1.3.6+) | Yes | All |
| announces_from_internal | Yes (1.3.6+) | Yes | All (default yes) |
| address / listen_ip | Yes | Yes | UDP, TCP server, QUIC/WebTransport/HTTPS server, DNSRendezvous |
| port / listen_port | Yes | Yes | UDP, TCP server, QUIC/WebTransport/HTTPS/VSOCK server, DNSRendezvous |
| target_host / target_port | Yes | Yes | TCP client, QUIC/WebTransport/HTTPS client |
| target_address | Yes | Yes | UDP peer (target_address over target_host) |
| interface | Yes | Yes | AutoInterface NIC name |
| kiss_framing | Yes | Parsed only | Reserved for KISS |
| i2p_tunneled | Yes | Yes | TCP client, backbone client |
| peers | Yes | Yes | I2PInterface outbound peers |
| connectable | Yes | Yes | I2PInterface SAM server tunnel |
| sam_address | Yes | Yes | I2PInterface SAM host:port |
| prefer_ipv6 | Yes | Yes | TCP, Auto |
| max_reconnect_tries | Yes | Yes | TCP, UDP, backbone, QUIC/WebTransport/HTTPS/VSOCK client. `-1` or omitted = unlimited |
| bitrate / mtu | Yes | Yes | All |
| discovery_port / data_port | Yes | Yes | Auto |
| discovery_scope | Yes | Yes | Auto |
| group_id / multicast_address_type | Yes | Yes | Auto |
| announce_cap, announce_rate_* | Yes | Yes | All |
| ingress_control, ic_* | Yes | Yes | All |
| outgoing / selected_outgoing | Yes | Yes | Receive-only when false. selected_outgoing is an alias |
| network_name / passphrase / ifac_* | Yes | Yes | Parsed and applied |
| cert_file / key_file | No | Yes | QUIC/WebTransport/HTTPS TLS PEM paths (optional, else ephemeral) |
| peer_key | No | Yes | QUIC/WebTransport/HTTPS SPKI SHA-256 pin (hex) |
| sni | No | Yes | QUIC/WebTransport/HTTPS client TLS ServerName |
| path | No | Yes | WebTransport/HTTPS URL path (default `/rns`) |
| transport_mode | No | Yes | WebTransport: datagram, stream, or dual |
| domain | No | Yes | DNSRendezvousInterface TXT lookup name |
| resolve_interval | No | Yes | DNSRendezvous re-query interval seconds |
| context_id / cid | No | Yes | VSOCKClientInterface peer CID |
| long_poll_sec | No | Yes | HTTPS long-poll timeout seconds |

## Protocol constants

| Item | Value |
|------|--------|
| Curve | Curve25519 (X25519 + Ed25519) |
| KEYSIZE | 512 bits (256-bit encryption + 256-bit signing) |
| TRUNCATED_HASHLENGTH | 128 bits |
| RATCHETSIZE | 256 bits |
| RATCHET_EXPIRY | 2 592 000 seconds (30 days) |
| Default MTU | 500 bytes (pkg/packet.MTU) |
