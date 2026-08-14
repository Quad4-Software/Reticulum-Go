# Links, channels, and resources

## Overview

Above raw destination packets, Reticulum provides encrypted links for session-oriented communication. On a link you can use:

- Request and response paths
- Reliable channel messages
- Stream buffers (bzip2 compressed)
- Multi-part resource (file) transfers

Packages: `pkg/link`, `pkg/channel`, `pkg/buffer`, `pkg/resource`.

## Links

A link is a bidirectional encrypted session between two destinations.

### Establishing links

**Outbound.** Application opens a link to a destination hash that is already known from an announce or path table. The initiator stores expected_hops from the path table (`PATHFINDER_M` when unknown). Link-request proofs are accepted only when the proof hop count matches, or when hops were unknown at creation (RNS 1.3.8).

**Inbound.** Peer sends a link request. Transport dispatches to:

```go
link.HandleIncomingLinkRequest(...)
```

The responder records expected_hops from the RTT packet hop field when the link becomes active.

There is no separate `Transport.CreateIncomingLink` helper. Incoming link requests use HandleIncomingLinkRequest directly.

### Link lifecycle

```
LINKREQUEST -> LINKIDENTIFY -> LINKREADY -> data / channel / resource
                  |
                  +-> teardown -> LINKCLOSE
```

Links track RTT, keepalive, and proof exchange per protocol rules. `Link.Reestablish` and WatchAndReconnect support automatic recovery when `Node.EnableLinkAutoReconnect` or equivalent options are set.

### Request and response

Register a request path on the link or destination. Handlers receive payload bytes and return a response. Timeouts apply if the handler blocks too long.

The control API bridges requests to WebSocket requestIncomingEvent and requestRespondCommand.

### Path recovery

`pkg/link/link_path_recovery.go` handles path loss during an active link session where the protocol allows recovery.

## Channel

`pkg/channel` provides reliable ordered message delivery inside a link.

| Concept | Description |
|---------|-------------|
| Channel | Session over an established link |
| Envelope | Wire wrapper for channel messages |
| MessageBase | Base type for message payloads |

Python 1.3.0 fixed ghost envelopes on failing outlets. Go matches that behavior: sequence allocation and tx-ring emplace happen only after a successful outlet send, with rewind on failure. Inbound envelopes use an RX ring so handlers run in sequence order with duplicates dropped, matching Python Channel._receive. Send refuses a full TX window and packed envelopes larger than the outlet MDU, matching Python Channel.send. Channel accepts both transport wrapper ACTIVE status and real link ACTIVE (`0x02`).

Typical pattern:

1. Establish link
2. Create channel on link
3. Send and receive Envelope messages

## Buffer

`pkg/buffer` implements a stream buffer over a channel.

Use cases:

- Streaming data larger than a single packet
- bzip2-compressed streams with bomb limits matching Python 1.1.9

Entry point:

```go
buf, err := buffer.CreateBidirectionalBuffer(link, ...)
```

Decompression limits protect against malicious compressed payloads. Same limits as Python reference.

## Resources

`pkg/resource` implements multi-part file transfer.

Features:

- Advertisement phase (Advertisement)
- Hash map of parts
- RESOURCE_PRF proof flow
- bzip2 compression optional (`bzip2_compress.go`)
- Split advertisements when the payload exceeds MaxEfficientSize (~1 MiB), matching Python segment chaining

Python utility rncp is ported as rgocp ([CLI utilities](utilities.md)). The primitives remain available in this package for Go applications.

See `examples/resources` for a minimal send/receive demo and `examples/filetransfer` for a directory browser.

Transfer flow at a high level:

```
Sender                           Receiver
  |                                  |
  |-- resource advertisement ------->|
  |<-- proof / acceptance -----------|
  |-- parts ------------------------>|
  |<-- proof ------------------------|
  |-- next segment (if split) ------>|
  |-- completion -------------------->|
```

## Interaction with transport

Link packets use PacketTypeLink. Data packets for link payloads use contexts handled in `Transport.handleTransportPacket` and forwardLinkData.

The transport link table tracks which local link object owns each session hash.

## Blackhole

Python 1.3.2 tears down links at LINKIDENTIFY when the remote identity is blackholed. Reticulum-Go does the same via `pkg/blackhole` during link identify. Federation publish and remote blackhole sources are not implemented. See [Compatibility](compatibility.md).

## Testing

| Area | Location |
|------|----------|
| Wire parity | `tests/crossref` (links, channel, buffer, resource) |
| Live link interop | `tests/interop/link_live_test.go` |
| Property tests | `pkg/buffer/*_pbt_test.go`, `pkg/resource/*_pbt_test.go` |
| Fuzz | `pkg/link/*_fuzz_test.go` |

## Application guidance

**Keep handlers short.** Request handlers run on link goroutines. Long work should offload to worker pools and respond before timeout.

**One link per peer session.** Multiplex logical streams with channel or buffer instead of opening redundant links.

**Wait with the stack, not a flat timer.** Outbound links need a path. Call `Transport.AwaitPath` (or send Control API `link.open` / librns `LinkOpen`) so the wait follows interface bitrate. Do not sleep 15 seconds.

**Resource size.** Respect MTU and part sizing. Large files use many parts over the same link.

## Related documents

- [API reference](api-reference.md)
- [Identity and destinations](identity-and-destinations.md)
- [Transport](transport.md)
- [Control API](control-api.md) for link commands over WebSocket
- [Examples](examples.md) for file transfer and link demos
- [CLI utilities](utilities.md) for rgocp
