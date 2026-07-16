# Interfaces

## Role

Interfaces move Reticulum packets between the transport layer and a physical or virtual medium. All implementations live in `pkg/interfaces` and conform to the `Interface` contract defined in `interface.go`.

Factory entry point:

```go
iface, err := interfaces.NewFromConfigWithContext(ctx, name, cfg)
```

## Interface contract

Each interface implements:

- `Start` and `Stop` for lifecycle
- `Send` for outbound data
- `ProcessIncoming` and `ProcessOutgoing` for IFAC and framing
- Statistics hooks used by the daemon and control API

Inbound path:

```
raw bytes -> ProcessIncoming -> packetCallback -> Transport.HandlePacket
```

Outbound path:

```
Transport.SendPacket -> Send -> ProcessOutgoing -> wire
```

## Supported types

| Config `type` | Status | File |
|---------------|--------|------|
| `UDPInterface` | Complete | `udp.go` |
| `TCPClientInterface` | Complete | `tcp.go` |
| `TCPServerInterface` | Complete | `tcp.go` |
| `AutoInterface` | Complete | `auto.go`, `auto_rescan.go`, `auto_roam.go` |
| `I2PInterface` | Complete | `i2p.go` (SAM in `pkg/i2p`) |
| `BackboneInterface` | Complete | `backbone.go` |
| `BackboneClientInterface` | Complete | `backbone_client.go` |
| `PipeInterface` | Complete | `pipe.go` |
| `LocalInterface` / `LocalServerInterface` | Complete | `local.go`, `sharedinstance` |
| `WebSocketInterface` | Go-only | `websocket_native.go`, `websocket_wasm.go` |
| `QUICClientInterface` | Go-only | `quic.go`, `quic_tls.go` |
| `QUICServerInterface` | Go-only | `quic.go`, `quic_tls.go` |

## Not implemented

These Python interface types have no driver in Reticulum-Go:

- RNodeInterface, RNodeMultiInterface
- SerialInterface, KISSInterface, AX25KISSInterface
- WeaveInterface
- Android-specific KISS, RNode, Serial variants

## PipeInterface

Bridges Reticulum to any external program over stdin/stdout using HDLC framing, matching Python `PipeInterface`.

Configuration:

- `command` (required): program and arguments, split like Python `shlex`
- `respawn_delay` (optional): seconds before respawning after subprocess exit (default 5)

## External interface plugins

Python loads `.py` modules from `{config_dir}/interfaces/`. Reticulum-Go keeps the same discovery path but uses process isolation and in-process factories instead of executing Python:

1. `interfaces.RegisterExternalFactory(typeName, factory)` for embedders
2. `{config_dir}/interfaces/{Type}.json` (or `.manifest`) with `driver` and `command` (pipe)
3. Executable `{config_dir}/interfaces/{Type}` used as a PipeInterface command

Example manifest:

```json
{"driver": "pipe", "command": "/usr/local/bin/my-rns-iface", "respawn_delay": 5}
```

Config:

```ini
[[Custom Radio]]
type = MyRadioIface
enabled = yes
```

## LocalInterface

Local shared-instance access uses HDLC over TCP (`127.0.0.1:port`) or abstract Unix (`@rns/<name>`).

Two configuration paths:

1. **Automatic (Python-compatible):** `share_instance = yes` in `[reticulum]` via `pkg/sharedinstance`
2. **Explicit interface block:** `type = LocalInterface` or `type = LocalServerInterface` in `[[...]]`

Local clients set `ConnectedToSharedInstance` and skip path-request ingress limiting, matching Python behavior.

## UDPInterface

Connects to a configured peer over UDP.

Requirements:

- Explicit `target_address` or `target_host` (same policy as Python `forward_ip`)
- Open binds do not adopt the source address of the first inbound packet

Optional reconnect when `max_reconnect_tries > 0` is a Go extension. Python does not reconnect UDP by default.

## TCP client and server

TCP uses HDLC framing with a maximum frame size cap (`maxHDLC` in `tcp.go`). Client interfaces support:

- Keepalives
- Reconnect via `reconnect.go`
- Tunnel re-synthesis on reconnect (`SetTunnelSynth` / `onConnected`)
- Optional I2P tunneling (`i2p_tunneled`)

Server interfaces accept inbound connections and apply IFAC on each session.

## AutoInterface

AutoInterface discovers peers on a local link using IPv6 link-local multicast.

Features:

- Peer aging and timeout
- Configurable discovery and data ports
- NIC binding via `interface` key
- Rescan when `watch_interfaces = yes` (`auto_rescan.go`)
- Listener replacement on Wi-Fi roam (`auto_roam.go`, aligned with Python 1.3.5)

## Interface modes

Modes match Python RNS wire values (`full` 0x01 through `internal` 0x07). Set with `mode` or `interface_mode` on an interface block.

| Mode | Effect (summary) |
|------|------------------|
| `full` | Default. Normal announce and path behavior |
| `access_point` | Does not rebroadcast announces |
| `gateway` / `roaming` / `internal` | Participate in unknown-path discovery (`DISCOVER_PATHS_FOR`) |
| `boundary` / `roaming` / `internal` | Extra announce forward filters vs next-hop mode (RNS 1.3.6+) |

`recursive_prs = yes` forces unknown-path discovery on any mode. `announces_from_internal = no` blocks rebroadcast of announces learned via an internal-mode next hop.

## I2PInterface

Uses I2P SAM (`pkg/i2p`) for inbound and outbound tunnels.

Configuration keys: `sam_address`, `peers`, `connectable`, `i2p_tunneled`.

Live tests require `RUN_LIVE_I2P=1` and a running SAM bridge.

## Backbone

Backbone interfaces multiplex many TCP streams through `pkg/backbone` hubs. Select poller backend with `backbone_io` in `[reticulum]`:

| Value | Platform |
|-------|----------|
| auto | Best available |
| epoll | Linux |
| kqueue | BSD, macOS |
| io_uring | Linux (when available) |
| go | Portable fallback |

## WebSocketInterface

Go-only transport for browser WASM clients. Native builds use `websocket_native.go`. WASM builds use `websocket_wasm.go`.

## QUICClientInterface / QUICServerInterface

Go-only QUIC transport (not available on WASM). HDLC frames ride one bidirectional QUIC stream per connection, matching the TCP interface framing model.

TLS follows a Yggdrasil-style mesh model:

- Ephemeral self-signed ECDSA P-256 certificates by default
- X.509 CA verification is skipped (`InsecureSkipVerify`)
- Optional `peer_key` pins the remote leaf SPKI SHA-256 (hex)
- Optional `cert_file` / `key_file` supply persistent PEM material
- Optional `sni` sets the client TLS ServerName
- ALPN is fixed to `rns`
- IFAC (`network_name` / `passphrase`) still applies above QUIC

```ini
[[QUIC Hub]]
type = QUICServerInterface
enabled = yes
listen_ip = 0.0.0.0
listen_port = 4242

[[QUIC Uplink]]
type = QUICClientInterface
enabled = yes
target_host = hub.example.com
target_port = 4242
peer_key = aabbccdd...
max_reconnect_tries = -1
```

Client reconnect uses `reconnect.go` like TCP. Server fan-out writes to all accepted sessions. Live Go-Go tests: `RUN_LIVE_INTEROP=1` with `tests/interop/quic_live_test.go`.

## Interface Access Code (IFAC)

When `network_name` and `passphrase` are set on an interface, frames are masked on egress and verified on ingress.

Policy:

- Wrong or missing IFAC on a configured interface results in silent drop on ingress
- Applied on UDP, TCP, Auto, and other supported types via `pkg/common.ApplyIFACInbound` and `ApplyIFACOutbound`

Details: [Cryptography](cryptography.md#ifac).

## Reconnect behavior

| Aspect | Python RNS | Reticulum-Go |
|--------|------------|--------------|
| TCP / backbone / QUIC client | Yes for TCP/backbone, 5 s wait | Yes via `reconnect.go` |
| I2P | Yes, 15 s wait | Yes in `i2p.go` |
| UDP | No | Yes when `max_reconnect_tries > 0` (opt-in) |
| Default max tries | Unlimited (`None`) | Unlimited (`-1` or omitted) |
| After exhaustion | Teardown | Teardown (`Stop`) |

`ConnectivityNotifier` hooks allow embedders to observe reconnect state (Go-only).

## Hot reload

`node.ReloadInterfaces` swaps interface blocks without restarting the process. On Unix, `SIGHUP` triggers reload in the daemon.

On unregister, transport scrubs paths, discovery state, announce bookkeeping, relay rows, and link-table entries for the removed interface.

Equality checks in `pkg/node/reload.go` cover type, addresses, I2P settings, IFAC, Auto ports, MTU, bitrate, prefer_ipv6, announce-rate, ingress/egress control, mode, and outgoing.

Tests: `interface_lifecycle_test.go`, `reload_e2e_test.go`.

## Rate and ingress settings

Per-interface keys `announce_cap`, `announce_rate_*`, `ingress_control`, and `ic_*` feed `pkg/rate` limiters consumed by transport ingress handlers.

## Operational notes

**MTU.** Default Reticulum packet MTU is 500 bytes (`pkg/packet.MTU`). Interface `mtu` should be consistent with the physical path.

**IPv6.** `prefer_ipv6` affects TCP and Auto binding and discovery.

**Panic on error.** `panic_on_interface_error = yes` can crash the daemon on fatal interface errors. Default is no.

## Testing

| Test | Env |
|------|-----|
| IFAC live | `RUN_LIVE_INTEROP=1`, `tests/interop/ifac_live_test.go` |
| Pipe live | `RUN_LIVE_INTEROP=1`, `tests/interop/pipe_live_test.go` |
| Shared RPC live | `RUN_LIVE_INTEROP=1`, `tests/interop/shared_rpc_live_test.go` |
| Auto live | `tests/interop/auto_live_test.go` |
| Backbone live | `tests/interop/backbone_live_test.go` |
| I2P live | `RUN_LIVE_I2P=1` |

## Related documents

- [Configuration](configuration.md) for interface block keys
- [Transport](transport.md) for registration and forwarding
- [Compatibility](compatibility.md) for Python interface matrix
