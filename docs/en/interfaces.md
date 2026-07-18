# Interfaces

## Role

Interfaces move Reticulum packets between the transport layer and a physical or virtual medium. All implementations live in `pkg/interfaces` and conform to the Interface contract defined in `interface.go`.

Factory entry point:

```go
iface, err := interfaces.NewFromConfigWithContext(ctx, name, cfg)
```

## Interface contract

Each interface implements:

- Start and Stop for lifecycle
- Send for outbound data
- ProcessIncoming and ProcessOutgoing for IFAC and framing
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

| Config type | Status | File |
|---------------|--------|------|
| UDPInterface | Complete | `udp.go` |
| TCPClientInterface | Complete | `tcp.go` |
| TCPServerInterface | Complete | `tcp.go` |
| AutoInterface | Complete | `auto.go`, `auto_rescan.go`, `auto_roam.go` |
| I2PInterface | Complete | `i2p.go` (SAM in `pkg/i2p`) |
| BackboneInterface | Complete | `backbone.go` |
| BackboneClientInterface | Complete | `backbone_client.go` |
| PipeInterface | Complete | `pipe.go` |
| SerialInterface | Complete | `serial.go` |
| LocalInterface / LocalServerInterface | Complete | `local.go`, sharedinstance |
| WebSocketInterface | Go-only | `websocket_native.go`, `websocket_wasm.go` |
| QUICClientInterface | Go-only | `quic.go`, `quic_tls.go` |
| QUICServerInterface | Go-only | `quic.go`, `quic_tls.go` |
| WebTransportClientInterface | Go-only | `webtransport.go` |
| WebTransportServerInterface | Go-only | `webtransport.go` |
| DNSRendezvousInterface | Go-only | `dns_rendezvous.go` |
| VSOCKClientInterface | Go-only (Linux) | `vsock.go` |
| VSOCKServerInterface | Go-only (Linux) | `vsock.go` |
| HTTPSClientInterface | Go-only | `https.go` |
| HTTPSServerInterface | Go-only | `https.go` |
| Modem73Interface | Complete | `modem73.go` |
| SDRInterface | Complete | `sdr.go`, `pkg/sdr` |

## Not implemented

These interface types have no driver in Reticulum-Go:

- RNodeInterface, RNodeMultiInterface
- KISSInterface, AX25KISSInterface
- WeaveInterface
- Android-specific KISS, RNode, Serial variants

## Modem73Interface

KISS TCP data path plus length-prefixed JSON control for a modem73 process.

Defaults: data `127.0.0.1:8001`, control `127.0.0.1:8073`, IFAC size 8.

```ini
[[MODEM73]]
type = Modem73Interface
enabled = yes
target_host = 127.0.0.1
target_port = 8001
control_host = 127.0.0.1
control_port = 8073
mtu_overhead = 15
bitrate = 400
auto_fragmentation = yes
short_frames = auto
short_mtu = 170
mode = boundary
```

DSP stays in the modem73 process. Live: `RUN_LIVE_INTEROP=1` with `tests/interop/modem73_live_test.go`.

Math-backed TNC simulator (no radio): Modem73Simulator in `modem73_sim.go` with PHY tables from [modem73](https://github.com/RFnexus/modem73) (`CONTROL_PORT.md`, robust nrows airtime, OFDM payload/bitrate/duration matrices). Shared Modem73Channel applies SNR and BER/FER models for multi-station tests.

## SDRInterface

Owns an SDR front end via pkg/sdr and a Go burst modem over baseband IQ.

**Lab / testing disclaimer.** This interface is for local mock, simulation, and controlled lab use. It is not a certified radio product and does not grant authority to transmit. Live TX (for example HackRF) can radiate on the tuned frequency. You must follow local license, band, power, and interference rules. Prefer device = mock or the math channel for development. The burst modem is not air-compatible with Modem73 OFDM or RNode. Everyday RF mesh should use purpose-built interfaces when those fit the link.

```ini
[[SDR0]]
type = SDRInterface
enabled = yes
device = mock
frequency = 433000000
sample_rate = 2000000
rx_gain = 20
tx_gain = 10
modem = burst
bitrate = 1200
mode = boundary
```

Device values: mock (always), rtltcp, rtlsdr (build tag sdr_rtlsdr), hackrf (build tag sdr_hackrf).

Default builds need no USB libraries. Burst modem is Go-native and is not air-compatible with Modem73 OFDM.

Math-backed RF channel: pkg/sdr/channel.go (FreeSpacePathLossDB, thermal noise, AWGN via Box-Muller, SimDevice). Validated by SNR measurement and FSPL exploratory checks.

Live: `RUN_LIVE_SDR=1` with `tests/interop/sdr_live_test.go`. Optional `SDR_DEVICE` / `SDR_ADDRESS` for hardware probes.

## SerialInterface

HDLC-framed serial port (MTU 564, baud as bitrate, default IFAC size 8).

Go extensions:

- Chunked reads instead of byte-at-a-time
- Configurable inter-byte frame idle drop (frame_idle_ms, default 100)
- max_reconnect_tries and reconnect delay
- Optional RTS/CTS DSR/DTR XON/XOFF keys
- IFAC and receive-only (`outgoing = no`)
- Live counters (FramesRX/TX, framing errors, reconnects)
- Injectable port opener for tests

```ini
[[Radio Serial]]
type = SerialInterface
enabled = yes
port = /dev/ttyUSB0
speed = 115200
databits = 8
parity = N
stopbits = 1
```

`device =` is an alias for the TTY path. Non-numeric `port =` values are treated as device paths so Python configs load.

Live Python framing interop: `RUN_LIVE_INTEROP=1` with `tests/interop/serial_live_test.go`.

## PipeInterface

Bridges Reticulum to any external program over stdin/stdout using HDLC framing.

Configuration:

- command (required): program and arguments, split like Python shlex
- respawn_delay (optional): seconds before respawning after subprocess exit (default 5)

## External interface plugins

Python loads `.py` modules from `{config_dir}/interfaces/`. Reticulum-Go keeps the same discovery path but uses process isolation and in-process factories instead of executing Python:

1. `interfaces.RegisterExternalFactory(typeName, factory)` for embedders
2. `{config_dir}/interfaces/{Type}.json` (or `.manifest`) with driver and command (pipe)
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

Local clients set ConnectedToSharedInstance and skip path-request ingress limiting, matching Python behavior.

## UDPInterface

Connects to a configured peer over UDP.

Requirements:

- Explicit target_address or target_host (same policy as Python forward_ip)
- Open binds do not adopt the source address of the first inbound packet

Optional reconnect when `max_reconnect_tries > 0` is a Go extension. Python does not reconnect UDP by default.

## TCP client and server

TCP uses HDLC framing with a maximum frame size cap (maxHDLC in `tcp.go`). Client interfaces support:

- Keepalives
- Reconnect via `reconnect.go`
- Tunnel re-synthesis on reconnect (SetTunnelSynth / onConnected)
- Optional I2P tunneling (i2p_tunneled)

Server interfaces accept inbound connections and apply IFAC on each session.

## AutoInterface

AutoInterface discovers peers on a local link using IPv6 link-local multicast.

Features:

- Peer aging and timeout
- Configurable discovery and data ports
- NIC binding via interface key
- Rescan when `watch_interfaces = yes` (`auto_rescan.go`)
- Listener replacement on Wi-Fi roam (`auto_roam.go`, aligned with Python 1.3.5)

## Interface modes

Modes match Python RNS wire values (full 0x01 through internal 0x07). Set with mode or interface_mode on an interface block.

| Mode | Effect (summary) |
|------|------------------|
| full | Default. Normal announce and path behavior |
| access_point | Does not rebroadcast announces |
| gateway / roaming / internal | Participate in unknown-path discovery (`DISCOVER_PATHS_FOR`) |
| boundary / roaming / internal | Extra announce forward filters vs next-hop mode (RNS 1.3.6+) |

`recursive_prs = yes` forces unknown-path discovery on any mode. `announces_from_internal = no` blocks rebroadcast of announces learned via an internal-mode next hop.

## I2PInterface

Uses I2P SAM (`pkg/i2p`) for inbound and outbound streams.

Outbound peers dial with a direct SAM `STREAM CONNECT` (no local TCP proxy hop). The peer is marked online only after that connect succeeds. Reconnect closes the prior SAM session and opens a new one. Stream sessions use `i2cp.leaseSetEncType=6,4` by default.

Configuration keys: sam_address, peers, connectable, i2p_tunneled.

Live SAM tests require `RUN_LIVE_I2P=1` and a running SAM bridge (`I2P_SAM_ADDRESS`, default `127.0.0.1:7656`). Directory peer smoke fetches online I2P hosts from `directory.rns.recipes` at runtime (override with `I2P_DIRECTORY_URL` or `INTEROP_DIRECTORY_URL`). Do not hardcode public b32 addresses in the tree.

Go and Python interop lives in `tests/interop/i2p_live_test.go` (`RUN_LIVE_INTEROP=1` plus a reachable SAM).

## Backbone

Backbone interfaces multiplex many TCP streams through `pkg/backbone` hubs. Select poller backend with backbone_io in `[reticulum]`:

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
- X.509 CA verification is skipped (InsecureSkipVerify)
- Optional peer_key pins the remote leaf SPKI SHA-256 (hex)
- Optional cert_file / key_file supply persistent PEM material
- Optional sni sets the client TLS ServerName
- ALPN is fixed to rns
- IFAC (network_name / passphrase) still applies above QUIC

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

## WebTransportClientInterface / WebTransportServerInterface

Go-only HTTP/3 WebTransport transport (not on WASM). Prefer datagrams for RNS packets. Optional HDLC stream mode matches QUIC.

- Default path `/rns`
- `transport_mode = datagram` (default), stream, or dual
- Application protocol rns
- Same TLS options as QUIC (cert_file, key_file, peer_key, sni)
- IFAC and reconnect supported

```ini
[[WT Hub]]
type = WebTransportServerInterface
enabled = yes
listen_ip = 0.0.0.0
listen_port = 4433
path = /rns
transport_mode = datagram

[[WT Client]]
type = WebTransportClientInterface
enabled = yes
target_host = hub.example.com
target_port = 4433
path = /rns
transport_mode = datagram
peer_key = aabbccdd...
```

## DNSRendezvousInterface

Go-only rendezvous underlay (not a DNS tunnel). Looks up DNS TXT for peer endpoints, then carries RNS packets over UDP to the discovered address.

TXT forms accepted:

- `rns=udp://1.2.3.4:4242`
- `rns proto=udp host=1.2.3.4 port=4242`

Hosts must be a parseable IP or DNS name without whitespace. Only udp and tcp schemes are accepted (udp is used by this underlay). Invalid TXT records are ignored.

```ini
[[DNS Peer]]
type = DNSRendezvousInterface
enabled = yes
domain = peers.example.com
listen_ip = 0.0.0.0
listen_port = 0
resolve_interval = 60
```

resolve_interval is seconds between re-queries (default 60). Publish the TXT at domain (or a name your resolver returns for that query). Live Go-Go: `RUN_LIVE_INTEROP=1` with `tests/interop/dns_rendezvous_live_test.go`.

## VSOCKClientInterface / VSOCKServerInterface

Go-only Linux `AF_VSOCK` transport with HDLC framing (same pattern as TCP). Useful for host-guest and same-host Local CID paths.

```ini
[[VSOCK Hub]]
type = VSOCKServerInterface
enabled = yes
port = 4242

[[VSOCK Client]]
type = VSOCKClientInterface
enabled = yes
context_id = 3
port = 4242
max_reconnect_tries = -1
```

context_id / cid is the peer CID (1 is Local on Linux). Not available on non-Linux or WASM. Live Local CID: `RUN_LIVE_INTEROP=1` with `tests/interop/vsock_live_test.go`.

## HTTPSClientInterface / HTTPSServerInterface

Go-only TLS long-poll packet underlay for restrictive networks where only HTTPS egress works. Not WebTransport or HTTP/3.

- Default path `/rns`
- Client `POST {path}/send` and long-poll `GET {path}/poll`
- Peer id header `X-RNS-Peer`
- Same TLS options as QUIC (cert_file, key_file, peer_key, sni)
- long_poll_sec default 25

```ini
[[HTTPS Hub]]
type = HTTPSServerInterface
enabled = yes
listen_ip = 0.0.0.0
listen_port = 8443
path = /rns
cert_file = /path/to/cert.pem
key_file = /path/to/key.pem

[[HTTPS Client]]
type = HTTPSClientInterface
enabled = yes
target_host = hub.example.com
target_port = 8443
path = /rns
long_poll_sec = 25
peer_key = aabbccdd...
```

Live Go-Go: `RUN_LIVE_INTEROP=1` with `tests/interop/https_live_test.go`.

## Interface Access Code (IFAC)

When network_name and passphrase are set on an interface, frames are masked on egress and verified on ingress.

Policy:

- Wrong or missing IFAC on a configured interface results in silent drop on ingress
- Applied on UDP, TCP, Auto, and other supported types via `pkg/common.ApplyIFACInbound` and ApplyIFACOutbound

Details: [Cryptography](cryptography.md#ifac).

## Reconnect behavior

| Aspect | Python RNS | Reticulum-Go |
|--------|------------|--------------|
| TCP / backbone / QUIC / WebTransport / HTTPS / VSOCK client | Yes for TCP/backbone, 5 s wait | Yes via `reconnect.go` |
| Serial | Yes, 5 s wait | Yes with max_reconnect_tries |
| I2P | Yes, 15 s wait | Yes in `i2p.go` |
| UDP / DNS rendezvous | No | UDP when `max_reconnect_tries > 0`. DNS re-resolves on interval |
| Default max tries | Unlimited (None) | Unlimited (`-1` or omitted) |
| After exhaustion | Teardown | Teardown (Stop) |

ConnectivityNotifier hooks allow embedders to observe reconnect state (Go-only).

## Hot reload

`node.ReloadInterfaces` swaps interface blocks without restarting the process. On Unix, `SIGHUP` triggers reload in the daemon.

On unregister, transport scrubs paths, discovery state, announce bookkeeping, relay rows, and link-table entries for the removed interface.

Equality checks in `pkg/node/reload.go` cover type, addresses, I2P settings, IFAC, Auto ports, MTU, bitrate, prefer_ipv6, announce-rate, ingress/egress control, mode, and outgoing.

Tests: `interface_lifecycle_test.go`, `reload_e2e_test.go`.

## Rate and ingress settings

Per-interface keys announce_cap, `announce_rate_*`, ingress_control, and `ic_*` feed `pkg/rate` limiters consumed by transport ingress handlers.

## Operational notes

**MTU.** Default Reticulum packet MTU is 500 bytes (`pkg/packet.MTU`). Interface mtu should be consistent with the physical path.

**IPv6.** prefer_ipv6 affects TCP and Auto binding and discovery.

**Panic on error.** `panic_on_interface_error = yes` can crash the daemon on fatal interface errors. Default is no.

## Testing

| Test | Env / command |
|------|---------------|
| IFAC live | `RUN_LIVE_INTEROP=1`, `tests/interop/ifac_live_test.go` |
| Pipe live | `RUN_LIVE_INTEROP=1`, `tests/interop/pipe_live_test.go` |
| Serial live | `RUN_LIVE_INTEROP=1`, `tests/interop/serial_live_test.go` |
| DNS rendezvous live | `RUN_LIVE_INTEROP=1`, `tests/interop/dns_rendezvous_live_test.go` |
| VSOCK live | `RUN_LIVE_INTEROP=1`, `tests/interop/vsock_live_test.go` (Linux) |
| HTTPS live | `RUN_LIVE_INTEROP=1`, `tests/interop/https_live_test.go` |
| QUIC / WebTransport live | `RUN_LIVE_INTEROP=1`, `tests/interop/quic_live_test.go` |
| Shared RPC live | `RUN_LIVE_INTEROP=1`, `tests/interop/shared_rpc_live_test.go` |
| Auto live | `tests/interop/auto_live_test.go` |
| Backbone live | `tests/interop/backbone_live_test.go` |
| I2P live | `RUN_LIVE_I2P=1` (directory peers fetched at runtime) |
| I2P Go and Python interop | `RUN_LIVE_INTEROP=1` plus reachable SAM, `tests/interop/i2p_live_test.go` |
| Race (Stop vs Send) | `go test -race ./pkg/interfaces/ -run Race` plus DNS/VSOCK/HTTPS/I2P unit tests |
| Goroutine leak | `go test ./pkg/interfaces/ -run NoGoroutineLeak` |
| Fuzz (examples) | `go test ./pkg/interfaces/ -run '^$' -fuzz=FuzzParseRNSTXT -fuzztime=20s` |

Fuzz targets cover TXT parsing (FuzzParseRNSTXT), HTTPS path/long-poll normalization, WebTransport path/mode, VSOCK CID and HDLC decode, Serial HDLC, peer-key pins, and I2P SAM message or destination resolve (`pkg/i2p`).

## Related documents

- [Configuration](configuration.md) for interface block keys
- [Transport](transport.md) for registration and forwarding
- [Compatibility](compatibility.md) for Python interface matrix
