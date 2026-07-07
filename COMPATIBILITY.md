# Compatibility with Python Reticulum

This document maps how Reticulum-Go compares to the official [Reticulum network API reference](https://reticulum.network/manual/reference.html) and the reference *rns* Python package (**RNS 1.3.5**).

Crossref tests clone the reference from `rns://7649a50d84610232d1416b41d2896aff/reticulum/reticulum` via [rngit](https://reticulum.network/manual/git.html) (`tests/crossref/run_crossref.sh`). The GitHub mirror at [markqvist/Reticulum](https://github.com/markqvist/Reticulum) is no longer used for vectors.

For crypto and storage see [docs/en/cryptography.md](docs/en/cryptography.md). For behaviour and threat model see [SECURITY.md](SECURITY.md). Full English docs: [docs/en/](docs/en/README.md).

## Table

| Component | Reticulum-Go | Notes |
|-----------|:------------:|-------|
| Crypto | Yes | Curve25519 (X25519 + Ed25519), AES-256-CBC, HMAC-SHA256, HKDF. Checked against Python in [tests/crossref](tests/crossref/). |
| Identity | Yes | Key generation, recall, sign/verify, encrypt/decrypt, ratchets. Optional 72-byte hardware-bound descriptor (RHB1). On-wire Ed25519 public key matches [RNS.Identity](https://github.com/markqvist/Reticulum/blob/master/RNS/Identity.py). Python `Identity.from_file` expects the 64-byte software layout only today. |
| Destination | Yes | SINGLE, GROUP, PLAIN, LINK. Announce and request handlers, links in and out. |
| Packet | Yes | Header types 1 and 2, all packet types and contexts. Byte-for-byte parity in crossref. |
| Transport | Yes | Core wire behaviour matches Python 1.3.5: path table, announces, RequestPath, hops, next-hop, type-2 rewrap, link-table forwarding, persistence, ingress control, and random-blob path selection (1.3.4 dedup). Optional gaps: transport probe responses (`respond_to_probes` / `allow_probes` config), and `Transport.CreateIncomingLink` helper stub (incoming links use `link.HandleIncomingLinkRequest` instead). |
| Interfaces | Partial | See [Interfaces](#interfaces) below. |
| Discovery (RNS.Discovery, rnstransport) | Partial | [pkg/discovery](pkg/discovery/) mirrors wire constants, LXStamper, msgpack layouts. `discover_interfaces` starts rnstransport listening via [pkg/node](pkg/node/) `StartInterfaceDiscovery`. InterfaceAnnouncer, BlackholeUpdater, and autoconnect loops are not auto-started. Build with `BuildAppData`, decode with `ValidateAndDecode`. Separate from AutoInterface multicast discovery. |
| Blackhole | Partial | [pkg/blackhole](pkg/blackhole/) covers table semantics, msgpack, expiry, MergeRemote, EncodeForRequest. Announces from listed identities are dropped. `/list` over rnstransport needs the RNS Request layer (not ported). **Gap (1.3.2):** Python tears down links at LINKIDENTIFY for blackholed identities. Go does not check in `HandleIdentification`. `publish_blackhole`, `blackhole_sources`, `blackhole_update_interval` are ignored. |
| IFAC | Yes | [pkg/ifac](pkg/ifac/) matches salt, HKDF identity, mask/unmask. UDP, TCP, Auto apply IFAC. Unauthenticated frames dropped. Live tests: [tests/interop/ifac_live_test.go](tests/interop/ifac_live_test.go). |
| Link | Yes | Both directions, RTT, request/response, channel, buffer, resources. `WatchAndReconnect` and `Node.EnableLinkAutoReconnect` use `Reestablish()` on closed links. |
| Resource | Yes | Multi-part transfer, hashmaps, RESOURCE_PRF, bzip2. BZ2 bomb limits match Python 1.1.9. |
| Channel | Yes | In-link reliable channel. [pkg/channel](pkg/channel/) tests. Python 1.3.0 fixed ghost envelopes. Go uses a simpler single-outlet model. |
| Buffer | Yes | Stream buffer over channel. [pkg/buffer](pkg/buffer/) tests. |
| Node lifecycle | Yes (Go-only) | [pkg/node](pkg/node/) embedder API: `OnNetworkAvailable`, `OnNetworkLost`, `RefreshPaths`, `ReloadInterfaces`, control API lifecycle routes. No Python equivalent. `watch_interfaces` polls NIC up/down and address changes via `net.Interfaces` on Linux, Android, Windows, macOS, and BSD (any CPU arch). Stub on WASM. `OnNetworkLost` may leave reconnect goroutines running until I/O fails. `ReloadInterfaces` equality skips some keys. See [Node lifecycle](#node-lifecycle-go-only). |

## Interfaces

Python: [RNS/Interfaces](https://github.com/markqvist/Reticulum/tree/master/RNS/Interfaces). Go: [pkg/interfaces](pkg/interfaces/).

| Python Reticulum | Reticulum-Go | Where / notes |
|------------------|:------------:|---------------|
| UDPInterface | Yes | [udp.go](pkg/interfaces/udp.go). IFAC via [pkg/common](pkg/common/). Requires explicit `target_host` or `target_address` (Python `forward_ip`). Open binds do not learn peers from the first packet. Optional reconnect when `max_reconnect_tries > 0` (Go extension). |
| TCPClientInterface | Yes | [tcp.go](pkg/interfaces/tcp.go), HDLC, keepalives. Reconnect in [reconnect.go](pkg/interfaces/reconnect.go). Re-synthesizes tunnels on reconnect (`SetTunnelSynth` / `onConnected`). |
| TCPServerInterface | Yes | Accept loop, HDLC, IFAC. |
| AutoInterface | Yes | [auto.go](pkg/interfaces/auto.go). IPv6 link-local multicast, peer aging. NIC rescan with `watch_interfaces` ([auto_rescan.go](pkg/interfaces/auto_rescan.go)). Roam listener swap ([auto_roam.go](pkg/interfaces/auto_roam.go), 1.3.5). |
| I2PInterface | Yes | [i2p.go](pkg/interfaces/i2p.go), SAM in [pkg/i2p](pkg/i2p/). Live tests: `RUN_LIVE_I2P=1`. |
| BackboneInterface | Yes | [backbone.go](pkg/interfaces/backbone.go), [backbone_client.go](pkg/interfaces/backbone_client.go). Multiplexed I/O in [pkg/backbone](pkg/backbone/). Live interop: [tests/interop/backbone_live_test.go](tests/interop/backbone_live_test.go). |
| RNodeInterface | No | No RNode serial driver. |
| RNodeMultiInterface | No | Depends on RNode driver. |
| SerialInterface | No | Not implemented. |
| KISSInterface | No | Not implemented. |
| AX25KISSInterface | No | Not implemented. |
| PipeInterface | No | Not implemented. |
| LocalInterface | Partial | Via `share_instance = yes` in [pkg/sharedinstance](pkg/sharedinstance/), [local.go](pkg/interfaces/local.go). |
| WeaveInterface | No | Not implemented. |
| Android KISS / RNode / Serial | No | Android-only. |
| Interface base | Yes | [interface.go](pkg/interfaces/interface.go), [constants.go](pkg/interfaces/constants.go). |
| WebSocket | Go-only | websocket_native.go, websocket_wasm.go. |

### Interface reconnect

| Aspect | Python *rns* | Reticulum-Go |
|--------|--------------|---------------|
| TCP / backbone client reconnect | Yes (`max_reconnect_tries`, 5 s wait) | Yes via [reconnect.go](pkg/interfaces/reconnect.go) |
| I2P peer reconnect | Yes (15 s fixed wait) | Yes ([i2p.go](pkg/interfaces/i2p.go)) |
| UDP reconnect | No | Yes when `max_reconnect_tries > 0` (Go extension, opt-in) |
| Default when `max_reconnect_tries` omitted | Unlimited (`None`) | Unlimited (`-1` via `NormalizeMaxReconnectTries`) |
| After max tries exhausted | Interface teardown | Teardown (`onExhausted` calls `Stop()`). Idle retry only if `allowIdleRetry` is set (off by default). |
| Connectivity hooks | N/A | `ConnectivityNotifier` for embedders |

### Interface hot reload (Go only)

Python has no equivalent. Convenience for long-running Go daemons.

`ReplaceInterface`, `UnregisterInterface`, and `SetReticulumConfig` swap interfaces without corrupting paths or links. Unregister clears paths, discovery state, announce bookkeeping, relay rows, and link-table entries for the removed iface.

[cmd/reticulum-go](cmd/reticulum-go/) wires `ReloadInterfaces`, config equality, SIGHUP reload, and coordinates `Stop()` with reload. Equality in [reload.go](pkg/node/reload.go) covers type, addresses, I2P, IFAC, and Auto ports. MTU, bitrate, `prefer_ipv6`, announce-rate, and ingress-control changes may not trigger rebuild.

Tests: `interface_lifecycle_test.go`, `interface_scrub_links_test.go`, `interface_stress_race_test.go`, `reload_e2e_test.go`.

## Node lifecycle (Go only)

Go-only embedder API in [pkg/node](pkg/node/) and the control API (`POST /v1/lifecycle/{resume,pause,refresh-paths}`). There is no Python equivalent. The API surface is complete for what Go ships. Platform limits are listed below.

| API | Behaviour |
|-----|-----------|
| `OnNetworkAvailable` | Clears link pause, rescans Auto NICs, starts offline interfaces, re-registers transport, optionally re-establishes watched links |
| `OnNetworkLost` | Sets link pause. Default mode calls `Disable()` on interfaces. Reconnect goroutines may run until I/O fails. |
| `RefreshPaths` | Expires stale paths and requests fresh paths for watched destinations and explicit API args |
| `ReloadInterfaces` | Hot-reloads `[[Interface]]` blocks from config |

`watch_interfaces` polls NIC state every 10s and calls `OnNetworkAvailable` on link or address changes (all desktop/mobile OS targets except WASM). Also enables AutoInterface NIC rescan. Python `discover_interfaces` starts rnstransport discovery (`StartInterfaceDiscovery`).

## Python 1.2.x to 1.3.5 changes vs Go

Wire format is unchanged in 1.2.x to 1.3.x. Most churn is utilities and transport behaviour.

| Python change | Version | Go status |
|---------------|---------|-----------|
| BZ2 decompression bomb limits | 1.1.9 | Covered |
| Path-request ingress/egress control | 1.2.5 | Covered (`pkg/rate`, `pkg/transport/ingress.go`) |
| Path table random-blob selection | 1.2.x+ | Covered (`pkg/transport/path_selection.go`) |
| Announce dedup when dest already in path table | 1.3.4 | Covered |
| Blackhole link teardown at LINKIDENTIFY | 1.3.2 | **Gap** (announce drop only) |
| AutoInterface link-local listener replacement on roam | 1.3.5 | Covered (`pkg/interfaces/auto_roam.go`) |
| Channel outlet ghost envelopes | 1.3.0 | Simpler Go channel model |
| Shared-instance RPC msgpack | 1.3.4 | Covered (`pkg/sharedinstance/rpc.go`) |
| `rngit` / `rnid` / `rnsh` utilities | 1.2.x+ | Not ported (no wire impact) |

## Security and robustness notes

| Topic | Python | Reticulum-Go |
|-------|--------|---------------|
| IFAC unauthenticated drop | Yes | Yes |
| Ingress/egress announce and PR rate limits | Yes (1.2.5) | Yes |
| BZ2 bomb limits on resource/buffer | Yes (1.1.9) | Yes |
| Blackholed identity announces | Dropped | Dropped |
| Blackholed identity links | Torn down at LINKIDENTIFY (1.3.2) | **Not checked** |
| UDP peer binding | Requires explicit `forward_ip` | Requires explicit `target_host` / `target_address` |
| HDLC frame size cap | Yes | Yes (`maxHDLC` in tcp.go) |

## Retained Go improvements

Intentional extensions beyond upstream *rns*:

| Area | Go behaviour |
|------|--------------|
| Path table persistence | RAM-only when no config path. Explicit opt-in for disk. |
| Interface hot reload | `ReloadInterfaces`, transport scrub on unregister |
| Node lifecycle API | `OnNetworkAvailable`, `OnNetworkLost`, `RefreshPaths`, control API |
| `watch_interfaces` | Linux/Android/Windows/macOS/BSD NIC poll plus AutoInterface rescan |
| UDP reconnect | Opt-in via `max_reconnect_tries > 0` |
| Backbone I/O | epoll/kqueue/io_uring multiplexing |
| WebSocket interface | Browser/WASM transport |
| Seccomp sandbox | Optional `enable_sandbox` in daemon |

## Utilities

| Python utility | Reticulum-Go | Notes |
|----------------|:------------:|-------|
| rnsd | Yes | [cmd/reticulum-go](cmd/reticulum-go/) daemon |
| rncp | No | Primitives in [pkg/resource](pkg/resource/) |
| rnid | No | Primitives in [pkg/identity](pkg/identity/) |
| rnir | No | Not ported |
| rnpath | No | Transport exposes RequestPath and table APIs |
| rnprobe | No | Not ported |
| rnstatus | No | Not ported |
| rnx | No | Not ported |
| rnodeconf | No | Depends on RNode driver |
| rnpkg | No | Not ported |
| rngit | No | Git-over-Reticulum. No wire impact. |
| rnsh | No | Not ported |
| WASM build | Go-only | [cmd/reticulum-wasm](cmd/reticulum-wasm/), [pkg/wasm](pkg/wasm/) |

## Examples

| Directory | Status | Notes |
|-----------|:------:|-------|
| wasm | Public | `//go:build js,wasm`. JS bridge via pkg/wasm. |
| pageserver | Public | Pages and static files over Reticulum. |
| announce, echo, filetransfer, link, minimal, page-downloader | Planned | N/A |

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
| Missing file | `__create_default_config__()` | `CreateDefaultConfig` in internal/config |

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
| logfile | When LOG_FILE | Unused. Logs to stderr. |
| storage/ | Container | Same |
| storage/identities/ | Per-name blobs | Per-hash blobs |
| storage/cache/ | General cache | Present |
| storage/cache/announces/ | Announce cache | Present |
| storage/resources/ | Resource scratch | Present |
| storage/blackhole | msgpack table | [pkg/blackhole](pkg/blackhole/) |
| storage/ratchets/ | Ratchet keys | Present |
| storage/destination_table | Path snapshot | Yes (flat msgpack, Python-compatible layout) |
| storage/known_destinations | Known destinations | Yes (loads Python byte-keyed files) |
| storage/transport_identity | Transport identity | Present |
| interfaces/ | Python plugin modules | Not supported |

### [reticulum] keys

| Key | Python *rns* | Reticulum-Go | Notes |
|-----|:------------:|:------------:|-------|
| enable_transport | Yes | Yes | Wired to Transport |
| share_instance | Yes | Yes | [pkg/sharedinstance](pkg/sharedinstance/). Server binds first, else client. |
| shared_instance_port | Yes | Yes | TCP port for shared instance |
| instance_control_port | Yes | Yes | RPC when this process owns the instance |
| instance_name | Yes | Yes | Unix socket name when `shared_instance_type = unix` |
| shared_instance_type | Yes | Yes | `tcp` or `unix` |
| backbone_io | No | Yes | Go-only. `auto`, `epoll`, `kqueue`, `io_uring`, or `go`. |
| rpc_key | No | Yes | Hex key for shared-instance RPC auth |
| enable_sandbox | No | Yes | Linux seccomp in daemon |
| enable_control_api | No | Yes | Localhost JSON API ([pkg/controlapi](pkg/controlapi/)) |
| control_api_host | No | Yes | Control API bind address |
| control_api_port | No | Yes | Control API port |
| panic_on_interface_error | Yes | Yes | Honoured on interface errors |
| in_memory_path_table | No | Yes | RAM-only path table |
| in_memory_known_destinations | No | Yes | RAM-only known destinations |
| discover_interfaces | Yes | Yes | Starts rnstransport listening (`StartInterfaceDiscovery`). Announcer not ported. |
| watch_interfaces | No | Yes | Go-only. Polls NIC changes via `net.Interfaces` (Linux, Android, Windows, macOS, BSD). WASM stub. Enables AutoInterface rescan. |
| respond_to_probes / allow_probes | Yes | No | Probe path not ported |
| publish_blackhole | Yes | No | Not auto-published |
| blackhole_sources | Yes | No | Ignored |
| blackhole_update_interval | Yes | No | Ignored (Python 1.3.2) |
| network_identity | Yes | No | Ignored |

### [logging] keys

| Key | Python *rns* | Reticulum-Go | Notes |
|-----|:------------:|:------------:|-------|
| loglevel | Yes | Yes | Same 0 to 7 scale |
| destination | Yes | No | Go logs to stderr only |

### [[Interface Name]] keys

Parsed in [pkg/reticulumconfig/config.go](pkg/reticulumconfig/config.go). Unlisted keys are ignored. IFAC options applied via `ApplyIFACFromConfig`.

| Key | Python *rns* | Reticulum-Go | Applies to (Go) |
|-----|:------------:|:------------:|-----------------|
| type | Yes | Yes | All |
| enabled / interface_enabled | Yes | Yes | All |
| address / listen_ip | Yes | Yes | UDP, TCP server |
| port / listen_port | Yes | Yes | UDP, TCP server |
| target_host / target_port | Yes | Yes | TCP client |
| target_address | Yes | Yes | UDP peer (`target_address` over `target_host`) |
| interface | Yes | Yes | AutoInterface NIC name |
| kiss_framing | Yes | Parsed only | Reserved for KISS |
| i2p_tunneled | Yes | Yes | TCP client, backbone client |
| peers | Yes | Yes | I2PInterface outbound peers |
| connectable | Yes | Yes | I2PInterface SAM server tunnel |
| sam_address | Yes | Yes | I2PInterface SAM host:port |
| prefer_ipv6 | Yes | Yes | TCP, Auto |
| max_reconnect_tries | Yes | Yes | TCP, UDP, backbone. `-1` or omitted = unlimited |
| bitrate / mtu | Yes | Yes | All |
| discovery_port / data_port | Yes | Yes | Auto |
| discovery_scope | Yes | Yes | Auto |
| group_id / multicast_address_type | Yes | Yes | Auto |
| announce_cap, announce_rate_* | Yes | Yes | All |
| ingress_control, ic_* | Yes | Yes | All |
| outgoing / selected_outgoing | Yes | No | Ignored |
| network_name / passphrase / ifac_* | Yes | Yes | Parsed and applied |

## Protocol constants

| Item | Value |
|------|--------|
| Curve | Curve25519 (X25519 + Ed25519) |
| KEYSIZE | 512 bits (256-bit encryption + 256-bit signing) |
| TRUNCATED_HASHLENGTH | 128 bits |
| RATCHETSIZE | 256 bits |
| RATCHET_EXPIRY | 2 592 000 seconds (30 days) |
| Default MTU | 500 bytes (pkg/packet.MTU) |
