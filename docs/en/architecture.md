# Architecture

## Purpose of this document

This page explains how Reticulum-Go is structured, describing layers, control flow, persistence, and deployment patterns.

For package-level detail see [Package map](package-map.md). For wire-level behavior see [Transport](transport.md) and [Interfaces](interfaces.md).

## Layered model

Reticulum-Go follows the same conceptual layers as Python Reticulum:

```
+-------------------------------------------------------------+
|  Application (your code, examples, control API clients)     |
+-------------------------------------------------------------+
|  Destination, Link, Resource, Channel, Buffer               |
+-------------------------------------------------------------+
|  Transport (routing, path table, announces, relay)        |
+-------------------------------------------------------------+
|  Interface (UDP, TCP, Auto, I2P, Backbone, WebSocket, QUIC) |
+-------------------------------------------------------------+
|  OS sockets, SAM, multicast, shared-instance multiplexer    |
+-------------------------------------------------------------+
```

Each layer depends downward only. Interfaces know nothing about application names. Destinations do not open raw sockets.

## Runtime components

### Daemon (`cmd/reticulum-go`)

The daemon is the long-running process most operators deploy. On startup it:

1. Loads configuration from `~/.reticulum-go/config` (or `--config`)
2. Creates a `node.Node` which owns transport and interfaces
3. Starts transport and registers each enabled interface
4. Optionally attaches to a shared instance (`share_instance`)
5. Applies the runtime sandbox (`pkg/sandbox`) unless disabled
6. Optionally starts the control API on localhost
7. Handles `SIGHUP` on Unix to hot-reload interface blocks

Shutdown on `SIGINT` or `SIGTERM` stops interfaces and flushes path persistence where configured.

### Node (`pkg/node`)

`Node` is the embedder-facing orchestration type. It wires:

- `transport.Transport` for routing
- `interfaces.Interface` instances from config
- `sharedinstance.Instance` when sharing one Reticulum process on a host
- Optional `discovery.InterfaceDiscovery` when `discover_interfaces` is enabled
- Network lifecycle hooks (`OnNetworkAvailable`, `OnNetworkLost`, `RefreshPaths`, `ReloadInterfaces`)

Library authors typically construct `node.New(cfg)` and call `Start()` rather than reimplementing transport registration.

### Transport (`pkg/transport`)

Transport is the routing engine. It maintains:

- A path table mapping destination hashes to next-hop interface and hop count
- Registered destinations and their packet receivers
- Link table entries for established sessions
- Announce handlers and ingress or egress rate limits
- Optional on-disk persistence for paths and known destinations

Every interface registers a callback that feeds inbound bytes into `Transport.HandlePacket`.

### Interfaces (`pkg/interfaces`)

Interfaces translate between Reticulum packets and a physical medium. They handle:

- IFAC mask and unmask on ingress and egress (`pkg/common`)
- HDLC framing for TCP and backbone clients
- Reconnect loops for TCP, UDP (opt-in), I2P, and backbone clients
- Platform-specific socket options (IPv6 preference, bind addresses)

Factory entry point: `interfaces.NewFromConfigWithContext`.

### Shared instance (`pkg/sharedinstance`)

When `share_instance = yes`, only one Reticulum process on a host should own the real interfaces. Other processes connect as clients over TCP or a Unix socket and multiplex packets through the owner. This mirrors Python behavior and uses msgpack RPC compatible with RNS 1.3.4 layouts.

Go CLI tools such as `rgostatus` dial this RPC. On Linux, Python `rnsd` defaults to a Unix abstract socket unless `shared_instance_type = tcp`. Setup for mixed Go and Python tooling is in [CLI utilities](utilities.md).

### Storage (`internal/storage`)

The daemon persists ratchets, identity blobs, destination tables, and related artifacts under `~/.reticulum-go/storage/`. Library embedders can use the same paths or keep tables in memory with `in_memory_path_table` and `in_memory_known_destinations`.

## Inbound packet flow

```
Wire bytes
    |
    v
Interface read loop
    |
    v
ProcessIncoming (IFAC verify, frame decode)
    |
    v
packetCallback -> Transport.HandlePacket
    |
    +-- PacketTypeAnnounce  -> path table update, handler dispatch
    +-- PacketTypeLink      -> link state machine
    +-- PacketTypeProof     -> proof handling
    +-- PacketTypeData      -> forward, relay, or deliver to Destination.Receive
```

Key functions (for code navigation):

- `BaseInterface.ProcessIncoming` in `pkg/interfaces/interface.go`
- `Transport.HandlePacket` in `pkg/transport/transport.go`
- `Transport.handleTransportPacket` for data context routing

## Outbound packet flow

```
Application sends via Destination or Link
    |
    v
Transport.SendPacket
    |
    v
Path lookup (may rewrap as header type 2 for multi-hop)
    |
    v
Packet.Serialize
    |
    v
Interface.ProcessOutgoing (IFAC mask)
    |
    v
Wire bytes
```

If no path exists, transport may emit path requests according to configuration and watched destinations.

## Concurrency model

- Each interface runs its own read loop (or shares a backbone hub poller).
- Transport uses internal locking and channels to serialize table updates and forwarding.
- Links run session goroutines for keepalive, request/response, and channel outlets.
- Hot reload takes `reloadMu` on `Node` to swap interfaces without tearing down unrelated state.

Backbone I/O can consolidate many TCP sockets behind one epoll, kqueue, or io_uring hub (`pkg/backbone`). Configure with `backbone_io` in `[reticulum]`.

## Deployment patterns

### Standalone node

One `reticulum-go` process with local interfaces. Suitable for gateways, radios, and servers.

```
[Internet or LAN] <--> UDP/TCP Interface <--> reticulum-go <--> Your app
```

### Shared instance

Multiple processes, one interface owner. Useful when a single radio or tunnel must be shared.

```
App A ----\
App B -----+---- TCP/Unix shared instance ---- reticulum-go (interface owner)
App C ----/
```

### Embedded library

A Go service links `pkg/node` directly. No daemon. The service loads config, starts `Node`, and registers destinations in-process.

### Browser WASM

`reticulum-wasm` compiles transport and a WebSocket interface. JavaScript calls `reticulum.init`, `connect`, `announce`, and related functions exposed by `pkg/wasm`.

### Control API sidecar

Non-Go applications talk HTTP and WebSocket to `pkg/controlapi` on localhost while the daemon owns transport. See [Control API](control-api.md).

### librns in-process

Native hosts link `librns.so` and call `include/rns.h`. Same stack as `pkg/node`, no separate daemon. Linux first. Odin hosts can use `bindings/odin`. See [librns](librns.md).

## Persistence and state

| State | Default location | Notes |
|-------|------------------|-------|
| Config | `~/.reticulum-go/config` | INI format, Python-compatible keys |
| Path table | `storage/destination_table` | Optional RAM-only mode |
| Known destinations | `storage/known_destinations` | Loads Python-format files |
| Identities | `storage/identities/` | Per-hash blobs |
| Ratchets | `storage/ratchets/` | Forward secrecy material |
| Blackhole table | `storage/blackhole` | msgpack |
| Transport identity | `storage/transport_identity` | Used when transport enabled |

## Security boundaries

Cryptography is centralized in `pkg/cryptography` and `pkg/identity`. IFAC adds an optional outer authentication layer on interface frames. The runtime sandbox limits filesystem and privilege exposure after startup. Neither replaces correct key handling or network segmentation.

See [Cryptography](cryptography.md) and [Security](security.md).

## Extension points

| Extension | Mechanism |
|-----------|-----------|
| Custom crypto for tests | `cryptography.SetProvider` |
| Hardware signing | `identity.NewIdentityWithSigner` with `cryptography.Ed25519Signer` |
| Embedder lifecycle | `node.Node` hooks and control API lifecycle routes |
| New interface types | Implement `interfaces.Interface`, register in `fromconfig.go` |
| Non-Go clients | Control API (out-of-process), librns (in-process C ABI), or `bindings/odin` |

Adding a new interface type or changing on-wire layouts requires coordinated updates across implementations and crossref vectors.

## What this stack does not include

- IP routing or DNS replacement
- Built-in application protocols (LXMF, Nomadnet, and similar live in separate projects)
- RNode firmware or serial radio drivers
- Post-quantum algorithms

Those may integrate over Reticulum destinations and links but are out of scope for this repository.
