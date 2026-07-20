# Configuration

## Overview

Reticulum-Go reads the same INI-style configuration shape as Python Reticulum. The canonical parser lives in `pkg/reticulumconfig`. The daemon imports it through `internal/config`.

Default config path:

```
~/.reticulum-go/config
```

Override with `--config` on the daemon command line.

## Config directory layout

```
~/.reticulum-go/
  config
  storage/
    identities/
    cache/
      announces/
    resources/
    ratchets/
    blackhole
    destination_table
    known_destinations/
    transport_identity
    dos_protect.mpack
```

Python uses `~/.reticulum` or `/etc/reticulum` by default. Reticulum-Go uses a separate directory so both stacks can run on one host.

## File format

| Aspect | Behavior |
|--------|----------|
| Sections | `[reticulum]`, `[logging]`, `[[Interface Name]]` |
| Interface blocks | Double-bracket headers at nesting depth 2 |
| Comments | `#` or `;` (including end-of-line after a space) |
| Booleans | yes, no, true, false, on, off, 1, 0 (case insensitive) |
| Unknown keys | Ignored (allows boot with extra or damaged lines) |
| UTF-8 BOM | Stripped on read |
| Missing file | Default config is created |

## Section `[reticulum]`

| Key | Default | Description |
|-----|---------|-------------|
| enable_transport | yes | Enable transport routing and path table |
| share_instance | yes | Use shared instance multiplexing |
| shared_instance_port | 37428 | TCP port for shared instance |
| instance_control_port | 37429 | Control port when this process owns the instance |
| shared_instance_type | unix on Linux, tcp elsewhere | tcp or unix (unset uses platform default, matching Python RNS) |
| instance_name | (empty) | Unix socket name when type is unix |
| rpc_key | (empty) | Hex key for shared-instance RPC and control API auth |
| enable_sandbox | yes | Apply OS sandbox after startup (Go-only) |
| enable_seccomp | yes | Linux seccomp-bpf denylist after Landlock (ignored when sandbox is off, soft-fails if unsupported) |
| enable_control_api | no | Start localhost control API |
| control_api_host | 127.0.0.1 | Control API bind address |
| control_api_port | 37430 | Control API port |
| backbone_io | auto | Backbone poller: auto, epoll, kqueue, io_uring, go |
| in_memory_path_table | no | Keep path table in RAM only |
| in_memory_known_destinations | no | Keep known destinations in RAM only |
| in_memory_storage | no | Fully ephemeral mode: no disk for paths, known dests, transport identity, blackhole, or split resources. Implies both table flags |
| identity_backend | file | Identity at-rest store: file, secretservice (Freedesktop Secret Service), or keyring (Linux kernel keyring) |
| soft_memory_limit | (none) | Soft heap budget (K/M/G or bytes) via Go `debug.SetMemoryLimit` |
| dos_protection | auto | Go-only local IDS/IPS gates. Values: off, detect, prevent, auto (alias smart). See [DoS protection](#dos_protection-go-only) |
| max_in_memory_paths | 100000 | Path table soft cap when in_memory_storage is yes. Negative disables |
| max_in_memory_known_destinations | 100000 | Known-dest soft cap when in_memory_storage is yes. Negative disables |
| max_in_memory_resource_bytes | 256M | Split-resource staging budget when in_memory_storage is yes. Negative disables |
| discover_interfaces | no | Start rnstransport interface discovery listener |
| watch_interfaces | no | Poll NIC up/down and rescan Auto interfaces (Go-only) |
| static_transport_identity | no | Keep persisted transport identity on the wire when enable_transport is no (RNS 1.3.6+) |
| local_hops_delta | no | Mangling applied on local-origin hop-0 packets (delta 2-7) |
| respond_to_probes / allow_probes | no | Register rnstransport.probe with prove-all |
| network_identity | (empty) | Path to network identity for discovery encrypt/decrypt |
| panic_on_interface_error | no | Panic on fatal interface errors |

### dos_protection (Go-only)

Local overload gates in `pkg/protect`. They keep **this node** alive under floods and resource storms. They do not ban peers mesh-wide and do not replace IFAC, link crypto, or Sybil-resistant admission policy.

| Mode | Behavior |
|------|----------|
| off | Disabled |
| detect (alias ids) | Trip, count, and rate-limited stdout warnings. Traffic still flows |
| prevent (aliases block, ips) | Same as detect, and shed or block on trip |
| auto (alias smart, default) | Learn quietly, then arm prevent. Relearn on interface fingerprint change or sustained moderate drift. Floods stay armed |

Example:

```
[reticulum]
  dos_protection = auto
```

Learning state is written as msgpack to `{config_dir}/storage/dos_protect.mpack` (atomic replace). Restarts restore armed baselines when the interface fingerprint still matches.

Surfaces gated when mode is not off:

| Gate | Typical attack class |
|------|----------------------|
| Per-iface packet pps / bps | Packet floods (adaptive EWMA baseline, once-per-second peak samples) |
| Handler overflow | Packet handler overload |
| Stream accepts | TCP / QUIC / VSOCK / I2P / Local connection storms |
| Incoming resources | Resource / transfer pile-up |
| Crypto jobs | Decrypt / verify / HMAC storms |
| Handshakes | Link setup / proof floods |
| Memory shed | Soft heap pressure (pairs with soft_memory_limit) |
| Iface cool-down | Temporary hard reject after a burst of trips on one iface |

Stdout trip lines look like:

```
WARNING: dos_protection prevent trip reason=pps iface=udp0
WARNING: dos_protection auto/learning trip reason=pps iface=udp0
WARNING: dos_protection auto promote reason=stable phase=armed
WARNING: dos_protection auto relearn reason=network phase=learning
```

Health kinds `dos_*` increment on trips. See [Security](security.md#dos-protection-local-idsips) and [Development and testing](development-and-testing.md#dos_protection-tests).

### Keys present in Python but ignored in Go

| Key | Notes |
|-----|-------|
| publish_blackhole | Blackhole auto-publish not started |
| blackhole_sources | Ignored |
| blackhole_update_interval | Ignored |

local_hops_delta applies a random hop field (2-7) on locally originated hop-0 packets when not connected to a shared instance.

## Section `[logging]`

| Key | Supported |
|-----|-----------|
| loglevel | Yes (0 through 7) |
| destination | Yes (stderr, file, both, syslog, journald, and combinations such as `syslog+stderr`) |
| logfile | Yes (default `{config_dir}/logfile/reticulum.log`) |
| format | Yes (text or json) |

## Interface blocks `[[Name]]`

Each block defines one interface. Common keys:

| Key | Applies to | Description |
|-----|------------|-------------|
| type | All | Interface type string (see [Interfaces](interfaces.md)) |
| enabled / interface_enabled | All | Enable or disable |
| mode / interface_mode | All | full, gateway, access_point, roaming, boundary, ptp, internal (RNS 1.3.6+) |
| recursive_prs | All | Discover unknown paths on this interface (RNS 1.3.6+) |
| announces_from_internal | All | Rebroadcast announces learned via internal-mode next hops (default yes) |
| address / listen_ip | UDP, TCP/QUIC/WebTransport/HTTPS server, DNSRendezvous | Bind address |
| port / listen_port | UDP, TCP/QUIC/WebTransport/HTTPS/VSOCK server, DNSRendezvous | Bind or VSOCK port |
| target_host / target_port | TCP, QUIC, WebTransport, HTTPS client | Remote peer |
| target_address | UDP | Remote peer (preferred over target_host) |
| interface | Auto | OS network interface name |
| prefer_ipv6 | TCP, Auto | Prefer IPv6 when available |
| max_reconnect_tries | TCP, UDP, backbone, QUIC, WebTransport, HTTPS, VSOCK | `-1` or omitted means unlimited |
| bitrate | All | Declared bitrate hint |
| mtu | All | Interface MTU (default packet MTU is 500 bytes) |
| discovery_port | Auto | Multicast discovery port |
| data_port | Auto | Data port |
| discovery_scope | Auto | Multicast scope |
| group_id | Auto | Group identifier |
| multicast_address_type | Auto | Multicast address type |
| announce_cap | All | Cap on outbound announces |
| `announce_rate_*` | All | Announce rate control |
| ingress_control / `ic_*` | All | Ingress rate limits |
| network_name | IFAC | IFAC network name |
| passphrase | IFAC | IFAC passphrase |
| `ifac_*` | IFAC | Additional IFAC options |
| sam_address | I2P | SAM server host:port |
| peers | I2P | Outbound tunnel peers |
| connectable | I2P | SAM server tunnel mode |
| i2p_tunneled | TCP client, backbone client | Tunnel over I2P |
| command | Pipe | External program for stdin/stdout HDLC bridge |
| respawn_delay / respawn_interval | Pipe | Seconds before respawning subprocess (default 5) |
| shared_instance_type | Local | tcp or unix for explicit local interface blocks |
| instance_name | Local | Unix socket name when type is unix |
| cert_file / key_file | QUIC, WebTransport, HTTPS | Optional TLS PEM paths |
| peer_key | QUIC, WebTransport, HTTPS | Leaf SPKI SHA-256 pin (hex) |
| sni | QUIC, WebTransport, HTTPS client | TLS ServerName |
| path | WebTransport, HTTPS | URL path (default `/rns`) |
| transport_mode | WebTransport | datagram, stream, or dual |
| domain | DNSRendezvous | DNS name for TXT lookup |
| resolve_interval | DNSRendezvous | Seconds between TXT re-queries (default 60) |
| context_id / cid | VSOCK client | Peer AF_VSOCK context ID |
| long_poll_sec | HTTPS | Long-poll timeout seconds (default 25) |
| outgoing / selected_outgoing | All | Transmit permit (default yes). When no, interface is receive-only |

Unknown type values load Go-native plugins from `{config_dir}/interfaces/` (JSON manifest or executable pipe driver), or from `interfaces.RegisterExternalFactory`.

## Interface types

| type value | Implementation |
|--------------|----------------|
| UDPInterface | `pkg/interfaces/udp.go` |
| TCPClientInterface | `pkg/interfaces/tcp.go` |
| TCPServerInterface | `pkg/interfaces/tcp.go` |
| AutoInterface | `pkg/interfaces/auto.go` |
| I2PInterface | `pkg/interfaces/i2p.go` |
| BackboneInterface | `pkg/interfaces/backbone.go` |
| BackboneClientInterface | `pkg/interfaces/backbone_client.go` |
| PipeInterface | `pkg/interfaces/pipe.go` |
| LocalInterface | `pkg/interfaces/local.go` (client to shared instance) |
| LocalServerInterface | `pkg/interfaces/local.go` (explicit server block) |
| WebSocketInterface | Go-only, native or WASM |
| QUICClientInterface | Go-only, native (`quic-go`) |
| QUICServerInterface | Go-only, native (`quic-go`) |
| WebTransportClientInterface | Go-only, HTTP/3 WebTransport |
| WebTransportServerInterface | Go-only, HTTP/3 WebTransport |
| DNSRendezvousInterface | Go-only, DNS TXT to UDP peer |
| VSOCKClientInterface | Go-only Linux, AF_VSOCK HDLC |
| VSOCKServerInterface | Go-only Linux, AF_VSOCK HDLC |
| HTTPSClientInterface | Go-only, TLS long-poll |
| HTTPSServerInterface | Go-only, TLS long-poll |

## Example: TCP client with IFAC

```ini
[[Gateway]]
type = TCPClientInterface
enabled = yes
target_host = mesh.example.com
target_port = 7825
network_name = MyMesh
passphrase = long-random-secret
ifac_size = 16
max_reconnect_tries = 0
```

`max_reconnect_tries = 0` means no reconnect attempts after disconnect. Omit the key or set `-1` for unlimited retries.

## Example: QUIC client and server

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
peer_key = aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899
max_reconnect_tries = -1
```

QUIC is Go-only. TLS uses ephemeral self-signed certificates by default. Set peer_key to the remote leaf SPKI SHA-256 (hex) to pin the peer. Optional cert_file / key_file and sni are supported. Not available on WASM.

## Example: AutoInterface on Wi-Fi

```ini
[reticulum]
watch_interfaces = yes

[[WiFi Mesh]]
type = AutoInterface
enabled = yes
interface = wlan0
discovery_port = 35615
data_port = 35616
```

watch_interfaces rescans NICs when link state changes and helps AutoInterface follow Wi-Fi roam events.

## Example: PipeInterface subprocess bridge

```ini
[[Radio Bridge]]
type = PipeInterface
enabled = yes
command = /opt/mesh/radio-bridge --stdio
respawn_delay = 5
```

Reticulum writes HDLC-framed packets to the subprocess stdin and reads frames from stdout. When the subprocess exits, the interface respawns after respawn_delay seconds.

## Example: shared-instance RPC for Go CLI tools

On Linux, unset shared_instance_type uses abstract Unix sockets (`@rns/<instance_name>/rpc`), matching stock Python rnsd. Go utilities try that default first, then fall back to TCP when the type is unset.

```bash
make build
./bin/reticulum-go status -config ~/.reticulum -json
```

Optional shared auth (recommended when mixing stacks):

```ini
[reticulum]
share_instance = yes
instance_name = default
shared_instance_type = unix
rpc_key = <64 hex characters>
```

Use `shared_instance_type = tcp` with instance_control_port when you want TCP on every OS. Use `-config ~/.reticulum` for Python rnsd and `-config ~/.reticulum-go` for a Go shared instance. Full utility docs are in [CLI utilities](utilities.md).

## Example: explicit LocalInterface client

When `share_instance = no`, attach to another process that owns the shared instance:

```ini
[reticulum]
share_instance = no

[[Local]]
type = LocalInterface
enabled = yes
port = 37428
shared_instance_type = tcp
```

## Hot reload

On Unix the daemon reloads interface blocks on `SIGHUP` via `node.ReloadInterfaces`. Reload compares normalized config for each interface including MTU, bitrate, prefer_ipv6, announce-rate, ingress/egress control, mode, and outgoing. See [Interfaces](interfaces.md).

## Programmatic access

```go
import "quad4/reticulum-go/pkg/reticulumconfig"

cfg, err := reticulumconfig.LoadConfig("/path/to/config")
```

Defaults without a file:

```go
cfg := reticulumconfig.DefaultConfig()
```

Save:

```go
err := reticulumconfig.SaveConfig(cfg)
```

## Storage and identity files

Identity files use a 64-byte software layout (X25519 private plus Ed25519 seed). Reticulum-Go stores identity blobs under `storage/identities/` keyed by hash. Python may use per-name files. Both layouts are supported for loading.

Optional hardware-bound descriptors (RHB1, 72 bytes) are documented in [Identity and destinations](identity-and-destinations.md).

## Related documents

- [Interfaces](interfaces.md) for per-type behavior and reconnect policy
- [Architecture](architecture.md) for shared instance and persistence
- [CLI utilities](utilities.md) for rgostatus / rgoid / rgoprobe and RPC setup
- [COMPATIBILITY.md](../../COMPATIBILITY.md) for full Python key comparison tables
