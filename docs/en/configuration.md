# Configuration

| Field | Value |
|-------|-------|
| Document version | 1.0 |
| Last updated | 2026-07-07 |
| Author | Ivan |

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
| `enable_transport` | yes | Enable transport routing and path table |
| `share_instance` | yes | Use shared instance multiplexing |
| `shared_instance_port` | 37428 | TCP port for shared instance |
| `instance_control_port` | 37429 | Control port when this process owns the instance |
| `shared_instance_type` | tcp | `tcp` or `unix` |
| `instance_name` | (empty) | Unix socket name when type is unix |
| `rpc_key` | (empty) | Hex key for shared-instance RPC and control API auth |
| `enable_sandbox` | yes | Apply OS sandbox after startup (Go-only) |
| `enable_control_api` | no | Start localhost control API |
| `control_api_host` | 127.0.0.1 | Control API bind address |
| `control_api_port` | 37430 | Control API port |
| `backbone_io` | auto | Backbone poller: auto, epoll, kqueue, io_uring, go |
| `in_memory_path_table` | no | Keep path table in RAM only |
| `in_memory_known_destinations` | no | Keep known destinations in RAM only |
| `discover_interfaces` | no | Start rnstransport interface discovery listener |
| `watch_interfaces` | no | Poll NIC up/down and rescan Auto interfaces (Go-only) |
| `static_transport_identity` | no | Keep persisted transport identity on the wire when `enable_transport` is no (RNS 1.3.6+) |
| `local_hops_delta` | no | Parsed for Python parity. Hop mangling not applied yet |
| `panic_on_interface_error` | no | Panic on fatal interface errors |

### Keys present in Python but ignored in Go

| Key | Notes |
|-----|-------|
| `respond_to_probes` | Transport probe responses not ported |
| `allow_probes` | Same |
| `publish_blackhole` | Blackhole auto-publish not started |
| `blackhole_sources` | Ignored |
| `blackhole_update_interval` | Ignored |
| `network_identity` | Ignored |

`local_hops_delta` is parsed and stored but hop mangling is not applied on the wire yet.

## Section `[logging]`

| Key | Supported |
|-----|-----------|
| `loglevel` | Yes (0 through 7) |
| `destination` | No (logs always go to stderr) |

## Interface blocks `[[Name]]`

Each block defines one interface. Common keys:

| Key | Applies to | Description |
|-----|------------|-------------|
| `type` | All | Interface type string (see [Interfaces](interfaces.md)) |
| `enabled` / `interface_enabled` | All | Enable or disable |
| `mode` / `interface_mode` | All | `full`, `gateway`, `access_point`, `roaming`, `boundary`, `ptp`, `internal` (RNS 1.3.6+) |
| `recursive_prs` | All | Discover unknown paths on this interface (RNS 1.3.6+) |
| `announces_from_internal` | All | Rebroadcast announces learned via internal-mode next hops (default yes) |
| `address` / `listen_ip` | UDP, TCP server | Bind address |
| `port` / `listen_port` | UDP, TCP server | Bind port |
| `target_host` / `target_port` | TCP client | Remote peer |
| `target_address` | UDP | Remote peer (preferred over target_host) |
| `interface` | Auto | OS network interface name |
| `prefer_ipv6` | TCP, Auto | Prefer IPv6 when available |
| `max_reconnect_tries` | TCP, UDP, backbone | `-1` or omitted means unlimited |
| `bitrate` | All | Declared bitrate hint |
| `mtu` | All | Interface MTU (default packet MTU is 500 bytes) |
| `discovery_port` | Auto | Multicast discovery port |
| `data_port` | Auto | Data port |
| `discovery_scope` | Auto | Multicast scope |
| `group_id` | Auto | Group identifier |
| `multicast_address_type` | Auto | Multicast address type |
| `announce_cap` | All | Cap on outbound announces |
| `announce_rate_*` | All | Announce rate control |
| `ingress_control` / `ic_*` | All | Ingress rate limits |
| `network_name` | IFAC | IFAC network name |
| `passphrase` | IFAC | IFAC passphrase |
| `ifac_*` | IFAC | Additional IFAC options |
| `sam_address` | I2P | SAM server host:port |
| `peers` | I2P | Outbound tunnel peers |
| `connectable` | I2P | SAM server tunnel mode |
| `i2p_tunneled` | TCP client, backbone client | Tunnel over I2P |
| `command` | Pipe | External program for stdin/stdout HDLC bridge |
| `respawn_delay` / `respawn_interval` | Pipe | Seconds before respawning subprocess (default 5) |
| `shared_instance_type` | Local | `tcp` or `unix` for explicit local interface blocks |
| `instance_name` | Local | Unix socket name when type is unix |

Keys such as `outgoing`, `selected_outgoing`, and `kiss_framing` are parsed or reserved but not wired for unsupported interface types.

## Interface types

| `type` value | Implementation |
|--------------|----------------|
| `UDPInterface` | `pkg/interfaces/udp.go` |
| `TCPClientInterface` | `pkg/interfaces/tcp.go` |
| `TCPServerInterface` | `pkg/interfaces/tcp.go` |
| `AutoInterface` | `pkg/interfaces/auto.go` |
| `I2PInterface` | `pkg/interfaces/i2p.go` |
| `BackboneInterface` | `pkg/interfaces/backbone.go` |
| `BackboneClientInterface` | `pkg/interfaces/backbone_client.go` |
| `PipeInterface` | `pkg/interfaces/pipe.go` |
| `LocalInterface` | `pkg/interfaces/local.go` (client to shared instance) |
| `LocalServerInterface` | `pkg/interfaces/local.go` (explicit server block) |
| `WebSocketInterface` | Go-only, native or WASM |
| `QUICClientInterface` | Go-only, native (`quic-go`) |
| `QUICServerInterface` | Go-only, native (`quic-go`) |

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

QUIC is Go-only. TLS uses ephemeral self-signed certificates by default. Set `peer_key` to the remote leaf SPKI SHA-256 (hex) to pin the peer. Optional `cert_file` / `key_file` and `sni` are supported. Not available on WASM.

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

`watch_interfaces` rescans NICs when link state changes and helps AutoInterface follow Wi-Fi roam events.

## Example: PipeInterface subprocess bridge

```ini
[[Radio Bridge]]
type = PipeInterface
enabled = yes
command = /opt/mesh/radio-bridge --stdio
respawn_delay = 5
```

Reticulum writes HDLC-framed packets to the subprocess stdin and reads frames from stdout. When the subprocess exits, the interface respawns after `respawn_delay` seconds.

## Example: shared-instance RPC for Go CLI tools

Python `rnsd` on Linux defaults to an abstract Unix RPC socket. Go `rgostatus` dials TCP `127.0.0.1:instance_control_port` when `shared_instance_type = tcp`. To let Go tools talk to Python (or the reverse), set the same TCP settings and preferably the same `rpc_key` in the daemon config:

```ini
[reticulum]
share_instance = yes
instance_name = default
shared_instance_type = tcp
shared_instance_port = 37428
instance_control_port = 37429
rpc_key = <64 hex characters>
```

Restart the daemon after changing these keys. Query with:

```bash
make build
./bin/reticulum-go status -config ~/.reticulum -json
```

Use `-config ~/.reticulum` for Python `rnsd` and `-config ~/.reticulum-go` for a Go shared instance. Full utility docs are in [CLI utilities](utilities.md).

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

On Unix the daemon reloads interface blocks on `SIGHUP` via `node.ReloadInterfaces`. Reload compares normalized config for each interface. Changes to MTU, bitrate, or some rate limits may not trigger a rebuild. See [Interfaces](interfaces.md).

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
- [CLI utilities](utilities.md) for `rgostatus` / `rgoid` / `rgoprobe` and RPC setup
- [COMPATIBILITY.md](../../COMPATIBILITY.md) for full Python key comparison tables
