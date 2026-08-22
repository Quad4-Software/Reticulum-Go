# Transport

## Role

`pkg/transport` is the routing engine. It learns paths from signed announces, forwards packets across interfaces, maintains link table state, and delivers data packets to registered destinations.

Transport sits between interfaces and application destinations. Every enabled interface registers with `Transport.RegisterInterface`, which sets a callback from inbound frames to HandlePacket.

Inbound packets are unpacked in `pkg/packet`. Hop counts `>= PATHFINDER_M` (128) are rejected at unpack time (RNS 1.3.8). Transport also drops announces and relayed packets that would exceed MaxHops after increment.
## Path table

The path table maps a 16-byte destination hash to:

- Next-hop interface name
- Hop count and timestamps
- Transport ID for multi-hop type-2 headers
- Interface-specific metadata

Paths are learned from:

- Signed announces received on interfaces
- Path response packets
- Explicit path requests from applications or the control API

### Path selection

When multiple paths exist, transport uses random-blob selection aligned with Python 1.3.4 deduplication behavior (`pkg/transport/path_selection.go`).

### Path requests

Applications call RequestPath on transport or use destination helpers. Ingress and egress controls (`pkg/rate`, `pkg/transport/ingress.go`) limit announce and path-request rates per interface configuration.

Unknown-path discovery (rebroadcasting a path request when no path is known) runs only when the receiving interface is in a discover mode (access_point, gateway, roaming, internal) or has `recursive_prs = yes` (RNS 1.3.6+).

Announce rebroadcast also applies interface mode filters (AP block, roaming/boundary/internal next-hop rules). See [Interfaces](interfaces.md).

### Persistence

By default the path table can persist to `storage/destination_table` as msgpack with a layout compatible with Python. Set `in_memory_path_table = yes` to keep paths in RAM only.

Known destinations persist under `storage/known_destinations/`. Set `in_memory_known_destinations = yes` for RAM-only mode.

Set `in_memory_storage = yes` for fully ephemeral stack state (paths, known destinations, transport identity, blackhole, and split-resource staging). Soft caps (`max_in_memory_paths`, `max_in_memory_known_destinations`, `max_packet_hashlist`, `soft_memory_limit`) bound RAM for path and known-dest tables and the packet hash filter even when storage is disk-backed.

## Packet handling

Inbound packets enter HandlePacket and branch on packet type:

```
HandlePacket
  |
  +-- Announce  -> handleAnnouncePacket
  |                 update path table, invoke announce handlers
  |
  +-- Link      -> handleLinkPacket
  |                 link setup, identify, keepalive
  |
  +-- Proof     -> handleProofPacket
  |
  +-- Data      -> handleTransportPacket
                    |
                    +-- ContextPathResponse -> handlePathResponse
                    +-- forwardTransportPacket (type-2 relay)
                    +-- forwardLinkData
                    +-- deliver to Destination.Receive
```

### Header type 2 relay

When transport mode is enabled and the local node acts as a relay, data packets may be rewrapped with header type 2 if the transport ID in the path matches. This matches Python transport relay semantics.

### Announce deduplication

If a destination is already present in the path table, duplicate announces may be dropped per Python 1.3.4 behavior.

## Links at the transport layer

Transport maintains a link table for active sessions. Link packets are routed to the correct `pkg/link` instance. Incoming link requests are dispatched to `link.HandleIncomingLinkRequest` (incoming link requests use `link.HandleIncomingLinkRequest`).

## Blackhole interaction

When `pkg/blackhole` has an entry for an identity hash, announces from that identity are dropped and links are torn down at LINKIDENTIFY. With `publish_blackhole = yes`, the daemon registers `rnstransport.info.blackhole` and serves `/list`. With `blackhole_sources` set, BlackholeUpdater periodically pulls remote lists and merges them. See [Compatibility](compatibility.md).

## Transport identity

When `enable_transport = yes`, transport may use a dedicated transport identity stored at `storage/transport_identity`. This identity participates in transport-mode relay and proofs as defined by the protocol.

A process that owns the shared instance still relays path requests, link requests, and link data for local clients when `enable_transport = no`. That matches Python RNS (`from_local_client` / `for_local_client_link`). Turning transport off only refuses to be a mesh hop for other nodes.

## Registration API

Typical embedder sequence:

```go
tr := transport.NewTransport(cfg)
tr.Start()

iface, err := interfaces.NewFromConfigWithContext(ctx, name, ifaceCfg)
tr.RegisterInterface(name, iface)
iface.Start()
```

RegisterInterface binds the interface packet callback to HandlePacket.

Outbound send path:

```go
err := tr.SendPacket(pkt)
```

SendPacket looks up the path, may rewrap for relay, serializes, and calls `Interface.Send` on the chosen interface.

## Rate limiting and ingress control

Per-interface ingress_control and `announce_rate_*` settings map to token buckets in `pkg/rate`. This mirrors Python 1.2.5 path-request and announce controls.

## Probes

Set `respond_to_probes = yes` (or allow_probes) to register `rnstransport.probe` with prove-all so `reticulum-go probe` can measure RTT against this node.

## Remote management

Set `enable_remote_management = yes` and `remote_management_allowed` to a comma-separated list of identity hashes. The daemon registers `rnstransport.remote.management` with `/path` (table, rates) and `/status` handlers, announced on the same interval as other management destinations. Python rnpath/rnstatus and Go rgopath/rgostatus both use that dest. Shared-instance clients do not host it. Remote drop and blackhole mutate are not implemented.

## Debugging

Raise loglevel in config. Transport logs at verbose (5) and above include path updates and forwarding decisions via `pkg/debug`.

## Testing

| Test area | Location |
|-----------|----------|
| Wire parity | `tests/crossref` (path requests, announces, relay) |
| Live relay | `tests/interop/transport_relay_live_test.go` (`RUN_LIVE_INTEROP=1`) |
| Live paths | `tests/interop/transport_path_live_test.go` |
| Property tests | `pkg/transport/*_pbt_test.go` |
| Race tests | `pkg/transport/*_race_test.go` |

## Related documents

- [Architecture](architecture.md) for end-to-end flow
- [Interfaces](interfaces.md) for physical egress
- [Links, channels, and resources](links-channels-and-resources.md) for session layer
- [Configuration](configuration.md) for transport-related keys
