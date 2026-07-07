# Compatibility with Python Reticulum

This is a practical map of how Reticulum-Go lines up with the official [Reticulum network API reference](https://reticulum.network/manual/reference.html) and the reference implementation in [markqvist/Reticulum](https://github.com/markqvist/Reticulum) (the *rns* Python package). See [docs/cryptography.md](docs/cryptography.md) for crypto and storage. See [SECURITY.md](SECURITY.md) for behaviour and the threat model.

## Table

| Component | Reticulum-Go | Notes |
|-----------|:------------:|-------|
| Crypto | Yes | Curve25519 (X25519 + Ed25519), AES-256-CBC (in Python, MODE_AES256_CBC is the only Link mode enabled today), HMAC-SHA256, and HKDF. Checked against Python in [tests/crossref](tests/crossref/). |
| Identity | Yes | Key generation, recall, sign or verify, encrypt or decrypt, and ratchets. Optional hardware-bound 72-byte descriptor (RHB1 header plus keys) through the identity package load and save paths. The on-wire Ed25519 public key matches [RNS.Identity](https://github.com/markqvist/Reticulum/blob/master/RNS/Identity.py), so Python peers verify announces unchanged. Python’s Identity.from_file today expects the 64-byte software layout only. The descriptor is documented here for future Python tooling. |
| Destination | Yes | SINGLE, GROUP, PLAIN, and LINK types. Announce and request handlers and link in or out. |
| Packet | Yes | Header types 1 and 2, all packet types and contexts, with byte-for-byte parity in crossref. |
| Transport | Yes | In-process transport covers the path table, announces, RequestPath, hop counts, next-hop selection, multi-hop type-2 rewrap, and link-table forwarding for link destinations. Path and known-destination tables persist to disk by default (Python-compatible flat msgpack under `storage/`) whenever a config path or `RETICULUM_STORAGE_PATH` is resolved. Use `in_memory_path_table`, `in_memory_known_destinations`, or `RETICULUM_IN_MEMORY_*` env vars to force RAM-only tables even with a config path; ad-hoc/library callers that never resolve a config path get RAM-only tables automatically, so embedding the transport never writes into a caller's home directory without an explicit path. Disk write/permission failures at any point fall back to RAM-only for the remainder of the process. |
| Interfaces | Partial | See [Interfaces](#interfaces) below. |
| Discovery (RNS.Discovery, rnstransport) | Partial | [pkg/discovery](pkg/discovery/) mirrors wire constants, LXStamper proof-of-work, and the msgpack info-dict and app_data layout, with tests against Python Discovery and LXStamper references. The high-level InterfaceAnnouncer and BlackholeUpdater loops from [RNS/Discovery.py](https://github.com/markqvist/Reticulum/blob/master/RNS/Discovery.py) are not auto-started. You build announces with BuildAppData and decode with ValidateAndDecode. This layer is separate from AutoInterface multicast discovery, which is supported. |
| Blackhole | Partial | [pkg/blackhole](pkg/blackhole/) covers Transport.blackholed_identities semantics, on-disk msgpack, expiry, MergeRemote, and EncodeForRequest. Transport drops announces from listed identities. The /list handler over rnstransport.info.blackhole needs the RNS Request layer, which is not ported. Python umsgpack round-trip is covered by pkg/blackhole interop tests. |
| IFAC | Yes | [pkg/ifac](pkg/ifac/) matches IFAC_SALT, HKDF-derived identity, and mask and unmask as in Python transmit and inbound paths. UDP, TCP, and AutoInterface apply outbound and inbound masking. Unauthenticated frames are dropped. See [tests/interop/ifac_live_test.go](tests/interop/ifac_live_test.go) for vectors and loopback against *rns*. |
| Link | Yes | Both directions, RTT, request and response, channel and buffer carriage, resource transfer. |
| Resource | Yes | Multi-part transfer, hashmaps, RESOURCE_PRF proofs, bzip2, accept and reject paths. |
| Channel | Yes | In-link reliable channel. Tests live under [pkg/channel](pkg/channel/). |
| Buffer | Yes | Stream buffer over channel. Tests live under [pkg/buffer](pkg/buffer/). |

## Interfaces

On the Python side, see [RNS/Interfaces](https://github.com/markqvist/Reticulum/tree/master/RNS/Interfaces). On the Go side, see [pkg/interfaces](pkg/interfaces/).

| Python Reticulum | Reticulum-Go | Where / notes |
|------------------|:------------:|---------------|
| UDPInterface | Yes | [udp.go](pkg/interfaces/udp.go). IFAC through [pkg/common](pkg/common/) ApplyIFAC outbound and inbound. |
| TCPClientInterface | Yes | [tcp.go](pkg/interfaces/tcp.go), tcp_common.go, HDLC, OS-specific keepalives (tcp_*_*.go). |
| TCPServerInterface | Yes | tcp.go accept loop, HDLC, IFAC. |
| AutoInterface | Yes | [auto.go](pkg/interfaces/auto.go). IPv6 link-local multicast, group hash, peer aging. |
| I2PInterface | Yes | [i2p.go](pkg/interfaces/i2p.go) with SAM bridge in [pkg/i2p](pkg/i2p/). Parent listener, outbound peers (`peers`), connectable server tunnel, spawned per-stream peers with HDLC, IFAC, ingress control, and tunnel synthesis. Requires a running I2P router with SAM. Live tests: `RUN_LIVE_I2P=1`. |
| BackboneInterface | Yes | Server: [backbone.go](pkg/interfaces/backbone.go) listens and spawns per-connection [BackboneClientInterface](pkg/interfaces/backbone_client.go) children (Python model). Client: outbound dial when `target_host` is set. Process-wide multiplexed I/O via [pkg/backbone](pkg/backbone/) (epoll on Linux, kqueue on BSD/macOS, `go` fallback). Optional `io_uring` backend probes the kernel and uses epoll-based multiplexing (no dedicated ring I/O yet). HDLC framing and backbone MTU/bitrate match Python. Live interop: [tests/interop/backbone_live_test.go](tests/interop/backbone_live_test.go). |
| RNodeInterface | No | Not implemented. There is no RNode serial driver. |
| RNodeMultiInterface | No | Depends on the RNode driver. |
| SerialInterface | No | Not implemented. |
| KISSInterface | No | Not implemented. |
| AX25KISSInterface | No | Not implemented. |
| PipeInterface | No | Not implemented (subprocess stdio bridge). |
| LocalInterface | Partial | Not a `[[Interface]]` type. Enabled via `share_instance = yes` in [pkg/sharedinstance](pkg/sharedinstance/) using [local.go](pkg/interfaces/local.go) LocalServerInterface / LocalClientInterface over TCP or Unix domain sockets with HDLC. Python-compatible RPC on `instance_control_port`. |
| WeaveInterface | No | Not implemented. |
| Android KISS / RNode / Serial | No | Android-only. Not part of the Go build. |
| Interface base | Yes | [interface.go](pkg/interfaces/interface.go) and [constants.go](pkg/interfaces/constants.go). |
| WebSocket | Go-only | websocket_native.go and websocket_wasm.go. Not in upstream *rns*. |

### Interface hot reload (Go only)

Python *rns* does not expose an equivalent. This is a convenience for long-running Go daemons.

The transport API for swapping or dropping interfaces without corrupting paths or link state includes ReplaceInterface, UnregisterInterface, and SetReticulumConfig. Unregister clears paths, discovery path state, announce bookkeeping, link relay rows, and link-table entries whose bound LinkInterface matches the removed iface. That uses Link.LinkedNetworkInterface on pkg/link.Link, and mocks must implement the same contract.

The [cmd/reticulum-go](cmd/reticulum-go/) daemon wires ReloadInterfaces, config equality for keep versus rebuild, SIGHUP reload on Unix, and coordinates Stop() with reload on one mutex.

Relevant tests: pkg/transport/interface_lifecycle_test.go, interface_scrub_links_test.go, and interface_stress_race_test.go (some heavy cases skip under -short), plus cmd/reticulum-go/reload_e2e_test.go for end-to-end reload.

## Utilities

| Python utility | Reticulum-Go | Notes |
|----------------|:------------:|-------|
| rnsd | Yes | [cmd/reticulum-go](cmd/reticulum-go/) is the daemon. It loads config, brings up interfaces, runs the transport, and persists identity and destination storage. |
| rncp | No | File copy CLI not ported. Resource primitives live in [pkg/resource](pkg/resource/). |
| rnid | No | Identity CLI not ported. Primitives live in [pkg/identity](pkg/identity/). |
| rnir | No | Identity recall CLI not ported. |
| rnpath | No | Path inspect CLI not ported. The transport still exposes RequestPath and table APIs. |
| rnprobe | No | Not ported. |
| rnstatus | No | Not ported. |
| rnx | No | Not ported. |
| rnodeconf | No | Depends on the RNode driver. |
| rnpkg | No | Not ported. |
| WASM build | Go-only | [cmd/reticulum-wasm](cmd/reticulum-wasm/). Browser stack through [pkg/wasm](pkg/wasm/). |

## Examples

Public samples live under [examples/](examples/). Planned examples are tracked for later publication.

| Directory | Status | Notes |
|-----------|:------:|-------|
| wasm | Public | //go:build js,wasm. JS bridge via pkg/wasm. |
| pageserver | Public | Pages and static files over Reticulum. |
| announce, echo, filetransfer, link, minimal, page-downloader | Planned | N/A |

## Configuration

Python defaults come from RNS.Reticulum.__create_default_config and the embedded template in [RNS/Reticulum.py](https://github.com/markqvist/Reticulum/blob/master/RNS/Reticulum.py) (search for __default_rns_config__), parsed with ConfigObj. Reticulum-Go parses the same shape in [internal/config](internal/config/config.go), which is canonical, with a thinner legacy path in [pkg/config](pkg/config/).

### Config file format

| Aspect | Python *rns* | Reticulum-Go |
|--------|--------------|---------------|
| Parser | configobj / ConfigObj | Hand-rolled scanner in internal/config |
| Top-level sections | [reticulum], [logging], [interfaces] | Same |
| Nested interface header | [[Interface Name]] (depth 2) | Same. Depth follows bracket count. Depth 2 or greater starts an interface. |
| Comments | Number sign (`#`) | Number sign and semicolon (`;`), including at end of line after a space |
| Booleans | Yes/No/True/False | yes/no/true/false/on/off/1/0 (case-insensitive) |
| Unknown keys / bad lines | ConfigObj errors | Ignored so a damaged file can still boot |
| UTF-8 BOM | Tolerated | Stripped at read |
| Missing file | `__create_default_config__()` | `CreateDefaultConfig` in internal/config |

### Config directory search path

Python tries system and home paths first. Go defaults to a single tree unless you pass --config.

```
# Python (typical order)
/etc/reticulum/config
~/.config/reticulum/config
~/.reticulum/config

# Reticulum-Go default
~/.reticulum-go/config    # use --config to point at a Python-style path
```

### Storage layout under the config directory

| Subpath | Python *rns* | Reticulum-Go |
|---------|----------------|---------------|
| config | Main file | Same (under ~/.reticulum-go when using the default layout) |
| logfile | When LOG_FILE | Unused. Logs go to stderr. |
| storage/ | Container | Same (internal/storage Manager basePath) |
| storage/identities/ | Per-name blobs | Per-hash blobs |
| storage/cache/ | General cache | Present |
| storage/cache/announces/ | Announce cache | Present |
| storage/resources/ | Resource scratch | Present |
| storage/blackhole | msgpack table | [pkg/blackhole](pkg/blackhole/) (path may differ) |
| storage/ratchets/ | Ratchet keys | Present |
| storage/destination_table | Path snapshot | Yes (flat msgpack list, Python-compatible entry layout; interface resolved by truncated hash of interface name) |
| storage/known_destinations | Known destinations | Yes (flat msgpack map; loads Python `known_destinations` byte-keyed files) |
| storage/transport_identity | Transport identity | Present |
| interfaces/ | Python plugin modules | Not supported |

### [reticulum] keys

| Key | Python *rns* | Reticulum-Go | Notes |
|-----|:------------:|:------------:|-------|
| enable_transport | Yes | Yes | Wired to Transport |
| share_instance | Yes | Yes | Wired in daemon via [pkg/sharedinstance](pkg/sharedinstance/). Server binds first; otherwise connects as client. |
| shared_instance_port | Yes | Yes | TCP listen/dial port for shared instance |
| instance_control_port | Yes | Yes | RPC server when this process owns the shared instance |
| instance_name | Yes | Yes | Unix socket name when `shared_instance_type = unix` |
| shared_instance_type | Yes | Yes | `tcp` or `unix` |
| backbone_io | No | Yes | Go-only. `auto`, `epoll`, `kqueue`, `io_uring`, or `go`. Selects [pkg/backbone](pkg/backbone/) multiplexer for backbone and local shared-instance sockets. |
| rpc_key | No | Yes | Hex key for shared-instance RPC authentication |
| enable_sandbox | No | Yes | Linux seccomp sandbox in daemon |
| enable_control_api | No | Yes | Localhost JSON control API ([pkg/controlapi](pkg/controlapi/)) |
| control_api_host | No | Yes | Bind address for control API |
| control_api_port | No | Yes | Port for control API |
| panic_on_interface_error | Yes | Yes | Honoured on interface errors |
| in_memory_path_table | No | Yes | When true, path table stays in RAM only |
| in_memory_known_destinations | No | Yes | When true, known destinations stay in RAM only |
| discover_interfaces | Yes | No | Ignored |
| respond_to_probes / allow_probes | Yes | No | Probe path not ported |
| publish_blackhole | Yes | No | Blackhole package does not auto-publish |
| blackhole_sources | Yes | No | Ignored |
| network_identity | Yes | No | Ignored |

### [logging] keys

| Key | Python *rns* | Reticulum-Go | Notes |
|-----|:------------:|:------------:|-------|
| loglevel | Yes | Yes | Same 0–7 scale |
| destination | Yes | No | Go logs to stderr only |

### [[Interface Name]] keys

Parsed by applyInterfaceOption in [pkg/reticulumconfig/config.go](pkg/reticulumconfig/config.go). Options not listed here (outgoing, selected_outgoing, and RNode or Serial-specific keys) are ignored at parse time. IFAC options (`network_name`, `passphrase`, `ifac_*`) are parsed and applied via ApplyIFACFromConfig.

| Key | Python *rns* | Reticulum-Go | Applies to (Go) |
|-----|:------------:|:------------:|-----------------|
| type | Yes | Yes | All |
| enabled / interface_enabled | Yes | Yes | All |
| address / listen_ip | Yes | Yes | UDP, TCP server |
| port / listen_port | Yes | Yes | UDP, TCP server |
| target_host / target_port | Yes | Yes | TCP client |
| target_address | Yes | Yes | UDP peer hint |
| interface | Yes | Yes | AutoInterface NIC name |
| kiss_framing | Yes | Parsed only | Reserved for KISS |
| i2p_tunneled | Yes | Yes | TCP client, backbone client |
| peers | Yes | Yes | I2PInterface outbound peer list |
| connectable | Yes | Yes | I2PInterface SAM server tunnel |
| sam_address | Yes | Yes | I2PInterface SAM host:port |
| prefer_ipv6 | Yes | Yes | TCP, Auto |
| max_reconnect_tries | Yes | Yes | TCP client |
| bitrate / mtu | Yes | Yes | All |
| discovery_port / data_port | Yes | Yes | Auto |
| discovery_scope | Yes | Yes | Auto (link / admin / site / organisation / global) |
| group_id / multicast_address_type | Yes | Yes | Auto |
| announce_cap, announce_rate_target, announce_rate_grace, announce_rate_penalty | Yes | Yes | All |
| ingress_control, ic_* burst/hold keys | Yes | Yes | All |
| outgoing / selected_outgoing | Yes | No | Ignored |
| network_name / passphrase / ifac_* | Yes | Yes | Parsed; applied via ApplyIFACFromConfig |

## Protocol constants

These match what RNS.Identity and RNS.Reticulum emit on the wire:

| Item | Value |
|------|--------|
| Curve | Curve25519 (X25519 + Ed25519) |
| KEYSIZE | 512 bits (256-bit encryption + 256-bit signing) |
| TRUNCATED_HASHLENGTH | 128 bits (identity and destination addressing) |
| RATCHETSIZE | 256 bits |
| RATCHET_EXPIRY | 2 592 000 seconds (30 days) |
| Default MTU | 500 bytes (pkg/packet.MTU) |
