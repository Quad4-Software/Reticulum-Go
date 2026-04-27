# Compatibility with the Python Reference

This document covers how well Reticulum-Go plays along with the official [Reticulum Network Stack API reference](https://reticulum.network/manual/reference.html) and the original Python `rns` package ([Reticulum](https://github.com/markqvist/Reticulum)). 

## Table

| Component                                        | Reticulum-Go | Notes |
|--------------------------------------------------|:------------:|-------|
| Crypto                                           | Yes          | Curve25519 (X25519 + Ed25519), AES-256-CBC (`MODE_AES256_CBC`, the only currently enabled `Link.ENABLED_MODES` entry in Python `rns`), HMAC-SHA256, HKDF; verified by `tests/crossref` |
| Identity                                         | Yes          | Key generation, recall, sign/verify, encrypt/decrypt, ratchets. Optional hardware-bound storage: 72-byte descriptor (`RHB1` v1 header + X25519 private + Ed25519 public) via `identity.LoadIdentityFile` / `ToHardwareBoundFile`; signing uses `Ed25519Signer` (same on-wire 64-byte public key as Python `RNS.Identity`, so Python peers verify announces without changes). Python `Identity.from_file` today only reads the 64-byte software layout; the descriptor format is documented here for a future Python loader or sidecar tooling. |
| Destination                                      | Yes          | `SINGLE`, `GROUP`, `PLAIN`, `LINK` types; announce, request handlers, link in/out |
| Packet                                           | Yes          | `HEADER_TYPE_1` and `HEADER_TYPE_2`, all packet types and contexts; byte-for-byte parity in crossref |
| Transport [^transport-scope]                     | Yes          | Path table, announces, `RequestPath`, hops, next-hop, multi-hop `HEADER_TYPE_2` rewrap, link-table forwarding for `DEST_TYPE_LINK` |
| Interfaces                                       | Partial      | See [Interfaces](#interfaces) section below |
| Discovery (`RNS.Discovery`, `rnstransport`)      | Partial      | `pkg/discovery` ports the wire constants, the LXStamper proof-of-work (`StampWorkblock`/`StampValue`/`StampValid`/`GenerateStamp`) and the msgpack info-dict / `app_data` flag-byte format. Cross-validated against the Python `RNS.Discovery` and `LXMF.LXStamper` references in `pkg/discovery` interop tests. The high-level `InterfaceAnnouncer` job loop and `BlackholeUpdater` from `RNS/Discovery.py` are not started automatically; callers compose announces with `BuildAppData` and decode with `ValidateAndDecode`. (Unrelated to `AutoInterface` multicast discovery, which is supported.) |
| Blackhole                                        | Partial      | `pkg/blackhole` implements `Transport.blackholed_identities`, on-disk msgpack persistence (matching `RNS.Transport.persist_blackhole`/`reload_blackhole`), expiry sweeping, per-source merge with `MergeRemote`, and `EncodeForRequest` (the payload that `Transport.blackhole_list_handler` returns). `Transport.HandleAnnounce` consults the table and drops announces from blackholed identities. The `/list` request handler over `rnstransport.info.blackhole` requires the RNS Request layer (not yet ported). Round-trip with Python `RNS.vendor.umsgpack` is verified by `pkg/blackhole` Python interop tests |
| IFAC (Interface Access Code)                     | Yes          | `pkg/ifac` ports `RNS.Reticulum.IFAC_SALT`, the HKDF-derived `ifac_identity`, and the byte-level mask/unmask used by `RNS.Transport.transmit`/`inbound`. UDP, TCP client/server and AutoInterface mask outbound packets and unmask inbound packets via `pkg/common.ApplyIFACOutbound`/`ApplyIFACInbound`, including the drop policy for missing or unauthenticated packets. Wire vectors and live UDP loopback against Python `rns` (`tests/interop/ifac_live_test.go`) confirm bidirectional interop and that unauthenticated peers are rejected |
| Link                                             | Yes          | Establish (both directions), RTT, requests/responses, channel and buffer carriage, resource transfer over link |
| Resource                                         | Yes          | Multi-part transfer, hashmaps, `RESOURCE_PRF` proofs, bzip2 compression, accept/reject (`ACCEPT_NONE`) |
| Channel                                          | Yes          | In-link reliable channel; unit tests in `pkg/channel` |
| Buffer                                           | Yes          | Stream buffer over channel; unit tests in `pkg/buffer` |

## Interfaces

| Python Reticulum          | Reticulum-Go | Go file(s) / notes |
|---------------------------|:------------:|--------------------|
| `UDPInterface`            | Yes          | `pkg/interfaces/udp.go`; IFAC mask/unmask via `pkg/common.ApplyIFACOutbound`/`ApplyIFACInbound` |
| `TCPClientInterface`      | Yes          | `pkg/interfaces/tcp.go` + `tcp_common.go`, HDLC framing, per-OS `tcp_<os>.go` keepalive tuning |
| `TCPServerInterface`      | Yes          | `pkg/interfaces/tcp.go`, accept loop, HDLC framing, IFAC support |
| `AutoInterface`           | Yes          | `pkg/interfaces/auto.go`; IPv6 link-local multicast group discovery, group hash, peer aging |
| `I2PInterface`            | No           | No Implemented Yet; no SAM bridge wrapper in Go |
| `BackboneInterface`       | No           | No Implemented Yet; the Python epoll/kqueue multiplexed backbone listener has no Go equivalent |
| `RNodeInterface`          | No           | No Implemented Yet; no RNode hardware/serial driver |
| `RNodeMultiInterface`     | No           | No Implemented Yet; depends on RNode driver |
| `SerialInterface`         | No           | No Implemented Yet |
| `KISSInterface`           | No           | No Implemented Yet |
| `AX25KISSInterface`       | No           | No Implemented Yet |
| `PipeInterface`           | No           | No Implemented Yet (subprocess stdio bridge) |
| `LocalInterface`          | No           | No Implemented Yet (Unix-domain shared-instance bridge) |
| `WeaveInterface`          | No           | No Implemented Yet |
| Android `KISS`/`RNode`/`Serial` (`RNS/Interfaces/Android/`) | No | Android-specific shims; not applicable to the Go build |
| `Interface` base class    | Yes          | `pkg/interfaces/interface.go` (`Interface` interface) + `constants.go` (mode/bitrate constants) |
| WebSocket interface       | Go-only      | `pkg/interfaces/websocket_native.go` and `websocket_wasm.go`; not present in Python `rns` |

### Interface hot reload (Reticulum-Go only)

Python `rns` today does not expose an equivalent: this is an implementation convenience in Go.

- **`transport.ReplaceInterface`**, **`transport.UnregisterInterface`**, and **`transport.SetReticulumConfig`** keep the transport and path state consistent when a logical interface is swapped or removed. Unregister scrubs paths, discovery path requests, announce/held-announce entries, link relay rows, and **`t.links`** entries whose `transport.LinkInterface` reports a matching bound iface via **`LinkedNetworkInterface()`** (implemented on `pkg/link.Link` and required on all `LinkInterface` mocks).
- **`cmd/reticulum-go`**: **`ReloadInterfaces`**, config equality for keep-vs-rebuild, **`SIGHUP`** reload on Unix, and **`Stop()`** serialized with the same mutex as reload to avoid races with teardown.
- **Tests**: `pkg/transport/interface_lifecycle_test.go`, `interface_scrub_links_test.go`, `interface_stress_race_test.go` (skipped with `-short` where heavy); end-to-end reload in `cmd/reticulum-go/reload_e2e_test.go`. Optional goroutine budget check: `RETICULUM_STRESS_LEAK=1 go test ./pkg/transport -run GoroutineBudget -count=3`. Recommended CI: `go test -race ./pkg/transport/...` (and broader packages as you already run).

## Utilities

| Python utility | Reticulum-Go | Notes |
|----------------|:------------:|-------|
| `rnsd`         | Yes          | `cmd/reticulum-go` is the daemon equivalent: loads config, brings up interfaces, runs the transport, persists identity/destination tables |
| `rncp`         | No           | Reticulum file copy CLI; No Implemented Yet (resource transfer primitives exist in `pkg/resource`) |
| `rnid`         | No           | Identity management CLI (create/inspect/sign/encrypt); No Implemented Yet (primitives in `pkg/identity`) |
| `rnir`         | No           | Identity recall / reverse-resolve CLI; No Implemented Yet |
| `rnpath`       | No           | Path table inspect / drop / request CLI; No Implemented Yet (transport exposes `RequestPath` and path table APIs) |
| `rnprobe`      | No           | Echo/probe CLI; No Implemented Yet |
| `rnstatus`     | No           | Interface and transport status CLI; No Implemented Yet |
| `rnx`          | No           | Remote command execution CLI; No Implemented Yet |
| `rnodeconf`    | No           | RNode flashing/configuration CLI; No Implemented Yet (depends on RNode driver) |
| `rnpkg`        | No           | Reticulum package signing/inspection CLI; No Implemented Yet |
| WASM build     | Go-only      | `cmd/reticulum-wasm` builds a browser-loadable Reticulum stack; no Python counterpart |

## Examples

Sample programs live under `examples/`. **Public** examples are included in the repository; **planned** examples exist in development and are intended for publication later.

| Directory | Status | Notes |
|-----------|:------:|-------|
| `wasm` | Public | Browser WASM (`//go:build js,wasm`), JS bridge via `pkg/wasm` |
| `pageserver` | Public | Serve templated pages and static files over Reticulum |
| `announce` | Planned | |
| `echo` | Planned | |
| `filetransfer` | Planned | |
| `link` | Planned | |
| `minimal` | Planned | |
| `page-downloader` | Planned | |

## Configuration

Reference: `RNS.Reticulum.__create_default_config` and the embedded `__default_rns_config__` template in `reticulum-ref/RNS/Reticulum.py` (around lines 1600-1715), parsed by `RNS.vendor.configobj`. Reticulum-Go re-implements the same nested-INI grammar in `internal/config/config.go` (canonical) and a thinner legacy parser in `pkg/config/config.go`.

### Config file format

| Aspect                                      | Python `rns`                          | Reticulum-Go                                    |
|---------------------------------------------|---------------------------------------|-------------------------------------------------|
| Parser                                      | `RNS.vendor.configobj` (ConfigObj)    | Hand-rolled scanner in `internal/config`        |
| Top-level sections                          | `[reticulum]`, `[logging]`, `[interfaces]` | Same                                       |
| Nested interface header                     | `[[Interface Name]]` (depth 2)        | Same; depth-by-bracket-count, depth >=2 is an interface |
| Comments                                    | `#`                                   | `#` and `;`, plus inline ` # ... ` / ` ; ... `  |
| Booleans                                    | `Yes`/`No`/`True`/`False`             | `yes`/`no`/`true`/`false`/`on`/`off`/`1`/`0`, case-insensitive |
| Unknown keys / malformed lines              | ConfigObj raises                      | Silently ignored so a damaged file still boots  |
| UTF-8 BOM at start of file                  | Tolerated by ConfigObj                | Stripped explicitly                              |
| Default file written when missing           | `__create_default_config()`           | `CreateDefaultConfig()` in `internal/config`     |

### Config directory search path

| Location                            | Python `rns` | Reticulum-Go | Notes |
|-------------------------------------|:------------:|:------------:|-------|
| `/etc/reticulum/config`             | Yes          | No           | Python tries this first if present |
| `~/.config/reticulum/config`        | Yes          | No           | Python's second choice |
| `~/.reticulum/config`               | Yes          | No           | Python's fallback |
| `~/.reticulum-go/config`            | No           | Yes          | Go's only default; pass `--config` to load a Python config file |

### Storage layout under the config directory

| Subpath               | Python `rns`                              | Reticulum-Go                                 |
|-----------------------|-------------------------------------------|----------------------------------------------|
| `config`              | Main config file                          | Same (under `~/.reticulum-go/`)              |
| `logfile`             | Log destination when `LOG_FILE`           | Not used; logging goes to stderr             |
| `storage/`            | Container directory                       | Same (`internal/storage.Manager.basePath`)   |
| `storage/identities/` | Per-name identity blobs                   | Per-hash identity blobs                       |
| `storage/cache/`      | Generic cache                             | Present                                       |
| `storage/cache/announces/` | Announce cache                       | Present                                       |
| `storage/resources/`  | In-flight resource scratch space          | Present                                       |
| `storage/blackhole`   | msgpack blackhole table                   | Handled by `pkg/blackhole` (separate path)    |
| `storage/ratchets/`   | Per-identity ratchet keys                 | Present                                       |
| `storage/destination_table` | Path table snapshot                 | Present (flat file, msgpack)                  |
| `storage/known_destinations` | Known-destination cache             | Present                                       |
| `storage/transport_identity` | Transport identity blob             | Present                                       |
| `interfaces/`         | Plugin interface Python modules           | Not supported; no plugin loader               |

### `[reticulum]` keys

| Key                          | Python `rns` | Reticulum-Go | Notes |
|------------------------------|:------------:|:------------:|-------|
| `enable_transport`           | Yes          | Yes          | Wired into `Transport` |
| `share_instance`             | Yes          | Parsed only  | No shared-instance RPC server in Go |
| `shared_instance_port`       | Yes          | Parsed only  | Stored on `ReticulumConfig` but no listener |
| `instance_control_port`      | Yes          | Parsed only  | Stored but no listener |
| `instance_name`              | Yes          | No           | Domain-socket multi-instance separator; ignored |
| `shared_instance_type`       | Yes          | No           | `tcp`/`unix` selector; ignored |
| `panic_on_interface_error`   | Yes          | Yes          | Honoured by interface error paths |
| `discover_interfaces`        | Yes          | No           | Network interface discovery; ignored |
| `respond_to_probes`          | Yes          | No           | Probe handler (`rnprobe`); ignored |
| `allow_probes`               | Yes          | No           | Probe permission flag; ignored |
| `publish_blackhole`          | Yes          | No           | `pkg/blackhole` does not auto-publish |
| `blackhole_sources`          | Yes          | No           | External blackhole list subscription; ignored |
| `network_identity`           | Yes          | No           | Pinned network identity; ignored |

### `[logging]` keys

| Key        | Python `rns` | Reticulum-Go | Notes |
|------------|:------------:|:------------:|-------|
| `loglevel` | Yes          | Yes          | Same 0-7 scale |
| `destination` | Yes       | No           | `stdout`/`file`/`syslog` selector; Go logs to stderr only |

### `[[Interface Name]]` keys

The keys below are parsed by `applyInterfaceOption` in `internal/config/config.go`. Keys not in the table (`outgoing`, `selected_outgoing`, `network_name`, `passphrase`, `ifac_size`, `ifac_netname`, `ifac_netkey`, plus any RNode/Serial/I2P-specific options) are silently ignored, so IFAC and other advanced settings have to be wired up programmatically.

| Key                            | Python `rns` | Reticulum-Go | Applies to (Go interfaces) |
|--------------------------------|:------------:|:------------:|----------------------------|
| `type`                         | Yes          | Yes          | All |
| `enabled` / `interface_enabled`| Yes          | Yes          | All |
| `address` / `listen_ip`        | Yes          | Yes          | UDP, TCP server |
| `port` / `listen_port`         | Yes          | Yes          | UDP, TCP server |
| `target_host`                  | Yes          | Yes          | TCP client |
| `target_port`                  | Yes          | Yes          | TCP client |
| `target_address`               | Yes          | Yes          | UDP (peer hint) |
| `interface`                    | Yes          | Yes          | AutoInterface (NIC name) |
| `kiss_framing`                 | Yes          | Parsed only  | Used by KISS interfaces (No Implemented Yet) |
| `i2p_tunneled`                 | Yes          | Parsed only  | Used by I2P (No Implemented Yet) |
| `prefer_ipv6`                  | Yes          | Yes          | TCP, AutoInterface |
| `max_reconnect_tries`          | Yes          | Yes          | TCP client |
| `bitrate`                      | Yes          | Yes          | All |
| `mtu`                          | Yes          | Yes          | All |
| `discovery_port`               | Yes          | Yes          | AutoInterface |
| `data_port`                    | Yes          | Yes          | AutoInterface |
| `discovery_scope`              | Yes          | Yes          | AutoInterface (`link`/`admin`/`site`/`organisation`/`global`) |
| `group_id`                     | Yes          | Yes          | AutoInterface |
| `multicast_address_type`       | Yes          | Yes          | AutoInterface |
| `announce_cap`                 | Yes          | Yes          | All |
| `announce_rate_target`         | Yes          | Yes          | All |
| `announce_rate_grace`          | Yes          | Yes          | All |
| `announce_rate_penalty`        | Yes          | Yes          | All |
| `ingress_control`              | Yes          | Yes          | All |
| `ic_new_time`                  | Yes          | Yes          | All |
| `ic_burst_freq_new`            | Yes          | Yes          | All |
| `ic_burst_freq`                | Yes          | Yes          | All |
| `ic_max_held_announces`        | Yes          | Yes          | All |
| `ic_burst_hold`                | Yes          | Yes          | All |
| `ic_burst_penalty`             | Yes          | Yes          | All |
| `ic_held_release_interval`     | Yes          | Yes          | All |
| `outgoing`                     | Yes          | No           | Per-interface outbound enable; ignored |
| `selected_outgoing`            | Yes          | No           | Outbound selection hint; ignored |
| `network_name`                 | Yes          | No           | IFAC virtual network name; ignored (use `pkg/ifac`) |
| `passphrase`                   | Yes          | No           | IFAC passphrase; ignored (use `pkg/ifac`) |
| `ifac_size`                    | Yes          | No           | IFAC mask size; ignored |
| `ifac_netname`                 | Yes          | No           | IFAC net name override; ignored |
| `ifac_netkey`                  | Yes          | No           | IFAC pre-shared key; ignored |

## Protocol Constants

We use the following constants on the wire to make sure we perfectly match what `RNS.Identity` and `RNS.Reticulum` expect:

| Item | Value |
|------|--------|
| Curve | Curve25519 (X25519 + Ed25519) |
| `KEYSIZE` | 512 bits (256-bit encryption key + 256-bit signing key) |
| `TRUNCATED_HASHLENGTH` | 128 bits (identity and destination addressing) |
| `RATCHETSIZE` | 256 bits |
| `RATCHET_EXPIRY` | 2 592 000 seconds (30 days) |
| Default MTU | 500 bytes (`pkg/packet.MTU`) |

