# Compatibility with Python Reticulum

This is a practical map of how Reticulum-Go lines up with the official [Reticulum network API reference](https://reticulum.network/manual/reference.html) and the reference implementation in [markqvist/Reticulum](https://github.com/markqvist/Reticulum) (the *rns* Python package). See [docs/cryptography.md](docs/cryptography.md) for crypto and storage. See [SECURITY.md](SECURITY.md) for behaviour and the threat model.

## Table

| Component | Reticulum-Go | Notes |
|-----------|:------------:|-------|
| Crypto | Yes | Curve25519 (X25519 + Ed25519), AES-256-CBC (in Python, MODE_AES256_CBC is the only Link mode enabled today), HMAC-SHA256, and HKDF. Checked against Python in [tests/crossref](tests/crossref/). |
| Identity | Yes | Key generation, recall, sign or verify, encrypt or decrypt, and ratchets. Optional hardware-bound 72-byte descriptor (RHB1 header plus keys) through the identity package load and save paths. The on-wire Ed25519 public key matches [RNS.Identity](https://github.com/markqvist/Reticulum/blob/master/RNS/Identity.py), so Python peers verify announces unchanged. Python’s Identity.from_file today expects the 64-byte software layout only. The descriptor is documented here for future Python tooling. |
| Destination | Yes | SINGLE, GROUP, PLAIN, and LINK types. Announce and request handlers and link in or out. |
| Packet | Yes | Header types 1 and 2, all packet types and contexts, with byte-for-byte parity in crossref. |
| Transport | Yes | In-process transport covers the path table, announces, RequestPath, hop counts, next-hop selection, multi-hop type-2 rewrap, and link-table forwarding for link destinations. |
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
| I2PInterface | No | Not implemented. There is no SAM bridge in Go. |
| BackboneInterface | No | Not implemented. Python’s epoll or kqueue backbone has no direct port. |
| RNodeInterface | No | Not implemented. There is no RNode serial driver. |
| RNodeMultiInterface | No | Depends on the RNode driver. |
| SerialInterface | No | Not implemented. |
| KISSInterface | No | Not implemented. |
| AX25KISSInterface | No | Not implemented. |
| PipeInterface | No | Not implemented (subprocess stdio bridge). |
| LocalInterface | No | Not implemented (Unix-domain shared instance). |
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
| announce, echo, filetransfer, link, minimal, page-downloader | Planned | — |

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
| storage/destination_table | Path snapshot | Present (flat msgpack) |
| storage/known_destinations | Known destinations | Present |
| storage/transport_identity | Transport identity | Present |
| interfaces/ | Python plugin modules | Not supported |

### [reticulum] keys

| Key | Python *rns* | Reticulum-Go | Notes |
|-----|:------------:|:------------:|-------|
| enable_transport | Yes | Yes | Wired to Transport |
| share_instance | Yes | Parsed only | No shared-instance RPC server in Go |
| shared_instance_port | Yes | Parsed only | Stored on config with no listener |
| instance_control_port | Yes | Parsed only | Stored on config with no listener |
| instance_name | Yes | No | Ignored. Unix multi-instance name separator. |
| shared_instance_type | Yes | No | Ignored. TCP versus Unix selector. |
| panic_on_interface_error | Yes | Yes | Honoured on interface errors |
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

Parsed by applyInterfaceOption in internal/config/config.go. Options not listed here (outgoing, selected_outgoing, network_name, passphrase, ifac_size, ifac_netname, ifac_netkey, and RNode, Serial, or I2P-specific keys) are ignored at parse time. IFAC and similar options still need programmatic wiring where a row says ignored.

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
| i2p_tunneled | Yes | Parsed only | Reserved for I2P |
| prefer_ipv6 | Yes | Yes | TCP, Auto |
| max_reconnect_tries | Yes | Yes | TCP client |
| bitrate / mtu | Yes | Yes | All |
| discovery_port / data_port | Yes | Yes | Auto |
| discovery_scope | Yes | Yes | Auto (link / admin / site / organisation / global) |
| group_id / multicast_address_type | Yes | Yes | Auto |
| announce_cap, announce_rate_target, announce_rate_grace, announce_rate_penalty | Yes | Yes | All |
| ingress_control, ic_* burst/hold keys | Yes | Yes | All |
| outgoing / selected_outgoing | Yes | No | Ignored |
| network_name / passphrase / ifac_* | Yes | No | Ignored in INI. Use pkg/ifac in code. |

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
