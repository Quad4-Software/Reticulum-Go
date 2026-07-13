# API reference

This is the application-facing API guide for Reticulum-Go. It is not a dump of every exported symbol. It is organized the way you build programs: choose an integration path, follow a recipe, then look up types and methods.

Wire behavior matches the [Python RNS API reference](https://reticulum.network/manual/reference.html). Package layout, concurrency rules, and embedder lifecycle are Go-specific and documented here because the Python manual does not cover them.

For generated signatures, use `go doc` on the import path or browse the module on pkg.go.dev. For package file maps, see [Package map](package-map.md).

## How this differs from the Python reference

| Python RNS manual | This document |
|-------------------|---------------|
| Class catalog (`RNS.Reticulum`, `Identity`, `Destination`, …) | Task-first recipes, then API tables |
| One process model (`RNS.Reticulum(...)`) | Four integration paths with trade-offs |
| Little concurrency guidance | Explicit callback and locking rules |
| No C / WASM / control-plane docs in the same place | Links to Control API, librns, WASM |
| Examples live elsewhere | Recipes point at `examples/` |

## Choose an integration path

```text
Need Reticulum in my app
            |
            v
      Go process?
       /        \
     yes         no
      |           |
      v           v
 pkg/node    Same machine as daemon?
 in-process        |
            -------+-------
           /       |       \
         yes    C/FFI   browser
          |       |       |
          v       v       v
     Control API  librns  pkg/wasm
     HTTP/WS      .so
          \       |       /
           \      |      /
            v     v     v
         destination + link
```

| Path | Package / surface | Use when |
|------|-------------------|----------|
| In-process Go | `pkg/node` | Default for Go services and tools |
| Daemon + JSON | [Control API](control-api.md) | Python, Rust, scripts, multi-language hosts |
| In-process C | [librns](librns.md) | Native hosts that cannot embed Go source |
| Browser | `pkg/wasm` | WebSocket gateway clients |

Most of this page describes the **`pkg/node` happy path**. Other paths expose the same concepts with different bindings.

## Mental model

1. **Config** loads interfaces and storage paths (`pkg/reticulumconfig`, `pkg/common`).
2. **Node** starts transport, interfaces, and optional shared instance (`pkg/node`).
3. **Identity** holds X25519 + Ed25519 keys (`pkg/identity`).
4. **Destination** is an app endpoint named `app.aspect…` (`pkg/destination`).
5. **Announce** publishes reachability. Peers learn paths.
6. **Path** is a cached route (`Transport.HasPath` / `RequestPath`).
7. **Link** is an encrypted session to a destination (`pkg/link`).
8. **Request / resource** move structured replies and large payloads (`Link.Request`, `pkg/resource`).

Packet MTU remains **500 bytes** on the wire (`pkg/packet.MTU`), same as Python.

## Quick start recipe (Go)

```go
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/reticulumconfig"
)

const appName = "example_utilities"

func main() {
	cfg, err := reticulumconfig.InitConfig()
	if err != nil {
		log.Fatal(err)
	}
	identity.InitKnownDestinationsPersistence(cfg.ConfigPath, cfg.InMemoryKnownDestinations)

	n, err := node.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := n.Start(); err != nil {
		log.Fatal(err)
	}
	defer n.Stop()

	id, err := identity.New()
	if err != nil {
		log.Fatal(err)
	}
	dest, err := destination.New(id, destination.In|destination.Out, destination.Single,
		appName, n.Transport(), "minimal")
	if err != nil {
		log.Fatal(err)
	}
	if err := dest.Announce(false, nil, nil); err != nil {
		log.Fatal(err)
	}
	log.Printf("listening on %x", dest.GetHash())

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
```

Runnable variants live under `examples/minimal`, `examples/announce`, `examples/echo`, and `examples/link`. See [Examples](examples.md).

## Recipe: inbound link and request handler

```go
dest.AcceptsLinks(true)
dest.SetLinkEstablishedCallback(func(v any) {
	l := v.(*link.Link) // import pkg/link
	_ = l.SetResourceStrategy(link.AcceptAll)
	l.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		log.Printf("data: %q", data)
	})
})

_ = dest.RegisterRequestHandler("/echo",
	func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) []byte {
		return data
	},
	destination.AllowAll, nil)
```

## Recipe: outbound link and request

```go
remoteID, err := identity.Recall(peerDestHash)
if err != nil {
	log.Fatal(err)
}
out, err := destination.FromHash(peerDestHash, remoteID, destination.Single, n.Transport())
if err != nil {
	log.Fatal(err)
}
if !n.Transport().HasPath(peerDestHash) {
	_ = n.Transport().RequestPath(peerDestHash, "", nil, false)
	// wait until HasPath is true
}
l := link.NewLink(out, n.Transport(), nil, nil, nil)
if err := l.Establish(); err != nil {
	log.Fatal(err)
}
receipt, err := l.Request("/echo", []byte("ping"), 15*time.Second)
if err != nil {
	log.Fatal(err)
}
// poll receipt.Concluded() or set receipt.SetResponseCallback
```

## Recipe: send a file resource

```go
res, err := resource.New(fileBytes, true)
if err != nil {
	log.Fatal(err)
}
_ = res.SetMetadata(map[string]any{"name": []byte("report.bin")})
if err := l.SendResource(res); err != nil {
	log.Fatal(err)
}
```

On the receiver, set `AcceptAll` or `AcceptApp` and handle `link.IncomingResource` (or plain `[]byte` when no metadata). CLI equivalent: `rgocp` in [CLI utilities](utilities.md).

## Recipe: network sleep and wake

```go
n.SetPauseMode(node.PauseModeDisable)
_ = n.OnNetworkLost()   // pause links, disable interfaces
_ = n.OnNetworkAvailable()
_ = n.RefreshPaths()    // re-request watched destinations
```

Optional: `n.EnableLinkAutoReconnect(node.LinkReconnectOptions{MaxAttempts: 5, Backoff: time.Second})` and `n.RegisterLink(l)`.

## Core types

### Node (`pkg/node`)

Orchestrates transport, interfaces, shared instance, and lifecycle. Prefer this over constructing `transport.Transport` by hand.

| Symbol | Role |
|--------|------|
| `New(cfg) (*Node, error)` | Build without starting |
| `Start() error` | Transport, path handler, shared instance, interfaces |
| `Stop() error` | Tear down in reverse order |
| `Transport() *transport.Transport` | Pass to destinations and links |
| `Config() *common.ReticulumConfig` | Active config |
| `Interfaces() []interfaces.Interface` | Configured interfaces |
| `OnNetworkAvailable() error` | Resume after outage |
| `OnNetworkLost() error` | Pause for sleep / NIC down |
| `SetPauseMode(PauseMode)` | `PauseModeDisable` or `PauseModeStop` |
| `WatchDestination(hash)` | Include hash in wake refreshes |
| `RefreshPaths(dests...)` | Force path refresh |
| `ReloadInterfaces(newCfg)` | Hot-reload interface blocks |
| `EnableLinkAutoReconnect(opts)` | Re-establish registered links |
| `RegisterLink(l)` | Track link for reconnect |
| `StartInterfaceDiscovery()` | rnstransport discovery when configured |

### Identity (`pkg/identity`)

| Symbol | Role |
|--------|------|
| `New() (*Identity, error)` | Generate software identity (preferred) |
| `NewIdentity()` | Alternate generator |
| `FromFile` / `ToFile` | 64-byte `[X25519 priv][Ed25519 seed]` |
| `FromBytes` / `FromPublicKey` | Load from bytes |
| `LoadIdentityFile(path, signer)` | Software or RHB1 hardware-bound |
| `NewIdentityWithSigner(...)` | External Ed25519 signer (HSM) |
| `Hash() []byte` | 16-byte truncated hash |
| `GetPublicKey() []byte` | 64-byte combined public key |
| `Sign` / `Verify` | Ed25519 |
| `Encrypt` / `Decrypt` | Identity tokens with optional ratchets |
| `Recall(destHash)` | Public identity from known destinations |
| `Remember` / `ValidateAnnounce` | Announce storage |
| `LoadOrCreateTransportIdentity` | Daemon transport identity |
| `RotateRatchet` / `GetRatchets` | Forward secrecy material |

Constants: `KeySize` (bits), `TruncatedHashLength` (bits). Hex destination or identity hashes are **32 characters**.

### Destination (`pkg/destination`)

| Constant | Meaning |
|----------|---------|
| `In` / `Out` | Direction bit flags (`In\|Out` for both) |
| `Single` / `Group` / `Plain` | Destination types |
| `ProveNone` / `ProveAll` / `ProveApp` | Proof strategy |
| `AllowNone` / `AllowAll` / `AllowList` | Request handler ACL |

| Symbol | Role |
|--------|------|
| `New(id, direction, type, app, transport, aspects...)` | Create and optionally auto-register (`In`) |
| `FromHash(hash, id, type, transport)` | Outbound destination for a known peer |
| `Hash(id, app, aspects...)` | Compute destination hash |
| `ParseName` / `ExpandAppName` | Dotted name helpers |
| `Announce(pathResponse, tag, iface)` | Publish reachability |
| `AcceptsLinks(bool)` | Accept link requests |
| `Encrypt` / `Decrypt` / `Sign` | Destination crypto |
| `SetPacketCallback` | Single-packet inbound data |
| `SetLinkEstablishedCallback` | Inbound link ready (`func(any)`) |
| `RegisterRequestHandler` / `RegisterRequestHandlerAny` | Link request paths |
| `EnableRatchets` / `RotateRatchets` | Group ratchet files |

### Link (`pkg/link`)

| Status | Value | Meaning on `Link` |
|--------|-------|-------------------|
| `StatusPending` | `0x00` | Not established |
| `StatusHandshake` | `0x01` | Handshake |
| `StatusActive` | `0x02` | Ready |
| `StatusStale` | `0x03` | Stale |
| `StatusClosed` | `0x04` | Closed |
| `StatusFailed` | `0x05` | Failed |

| Symbol | Role |
|--------|------|
| `NewLink(dest, transport, iface, onEst, onClose)` | Outbound link object |
| `Establish() error` | Initiator handshake |
| `Teardown()` | Close |
| `Identify(id)` | Prove local identity to peer |
| `Send` / `SendPacket` / `SendPacketWithContext` | Encrypted data |
| `Request(path, data, timeout)` | Msgpack request (auto resource if large) |
| `SendResource(res)` | Outbound resource transfer |
| `GetChannel()` | Reliable channel over the link |
| `SetResourceStrategy` | `AcceptNone` / `AcceptAll` / `AcceptApp` |
| `SetResourceConcludedCallback` | `[]byte` or `IncomingResource` |
| `GetRTT` / idle timers / PHY stats | Link health |

#### RequestReceipt

| Method | Role |
|--------|------|
| `Concluded()` | Finished (success or failure) |
| `GetStatus()` | **`StatusActive` means response OK**, `StatusFailed` means timeout or error |
| `GetResponse()` / `GetResponseValue()` | Bytes or decoded msgpack |
| `GetMetadata()` | Resource response metadata |
| `Progress()` | Bytes received / total for resource replies |
| `SetResponseCallback` / `SetFailedCallback` | Async completion |

Do not confuse `RequestReceipt.GetStatus()` with `Link.GetStatus()`. Both reuse status byte constants with different meanings.

### Resource (`pkg/resource`)

| Symbol | Role |
|--------|------|
| `New(data, autoCompress)` | `[]byte` or seekable file |
| `SetMetadata(map)` | Prepended msgpack metadata (Python-compatible) |
| `GetProgress` / `GetStatus` / `GetHash` | Transfer state |
| `PrepareOutboundForLink` | Called by `Link.SendResource` |

Statuses: `StatusPending`, `StatusActive`, `StatusComplete`, `StatusFailed`, `StatusCancelled`.

### Transport (via `Node.Transport()`)

| Method | Role |
|--------|------|
| `HasPath(hash)` | Cached route present |
| `RequestPath(hash, iface, tag, recursive)` | Path request (throttled) |
| `HopsTo` / `NextHop` / `NextHopInterface` | Route inspection |
| `ExpirePath` / `PrepareFreshPathRequest` | Drop or refresh cache |
| `RegisterInterface` / `GetInterfaces` | Interface table |
| `RegisterDestination` | Usually automatic for `In` destinations |
| `SendPacket` / `HandlePacket` | Low-level inject (advanced) |
| `RegisterAnnounceHandler` | Observe announces |

Avoid `transport.Destination` and `transport.Link` placeholder types. Use `destination.Destination` and `link.Link`.

### Packet (`pkg/packet`)

| Symbol | Role |
|--------|------|
| `MTU` | 500 |
| `NewPacket` / `Pack` / `Unpack` | Wire encode/decode |
| `PacketReceipt` | Delivery proofs for data packets |
| Context constants | `ContextRequest`, `ContextResource`, link contexts, … |

### Config (`pkg/reticulumconfig`, `pkg/common`)

| Function | Role |
|----------|------|
| `InitConfig()` | Load or create `~/.reticulum-go/config` |
| `LoadConfig(path)` | Parse INI (unknown keys ignored) |
| `SaveConfig` / `DefaultConfig` / `CreateDefaultConfig` | Persist defaults |

Important `ReticulumConfig` fields: `EnableTransport`, `ShareInstance`, `SharedInstanceType`, ports, `RPCKey`, `Interfaces`, `EnableControlAPI`, `InMemoryPathTable`, `WatchInterfaces`, `DiscoverInterfaces`, `BackboneIO`.

Default config directory is **`~/.reticulum-go`**, not `~/.reticulum`.

## Python to Go map

| Python | Go |
|--------|-----|
| `RNS.Reticulum(configdir=...)` | `reticulumconfig.LoadConfig` + `node.New` + `Start` |
| `RNS.Identity()` | `identity.New()` |
| `Identity.from_file` / `to_file` | `FromFile` / `ToFile` |
| `Identity.recall(hash)` | `identity.Recall(hash)` |
| `Destination(identity, IN, SINGLE, app, *aspects)` | `destination.New(id, destination.In, destination.Single, app, tr, aspects...)` |
| `Destination(..., OUT, ...)` | `destination.Out` or `FromHash` for known peers |
| `destination.announce()` | `dest.Announce(false, nil, nil)` |
| `destination.set_link_established_callback` | `SetLinkEstablishedCallback` (`func(any)`) |
| `destination.register_request_handler` | `RegisterRequestHandler` / `RegisterRequestHandlerAny` |
| `RNS.Link(destination)` | `link.NewLink` + `Establish` |
| `link.identify(identity)` | `l.Identify(id)` |
| `link.request(path, data=...)` | `l.Request(path, data, timeout)` |
| `RNS.Resource(data, link, metadata=...)` | `resource.New` + `SetMetadata` + `l.SendResource` |
| `RNS.Transport.has_path` / `request_path` | `tr.HasPath` / `tr.RequestPath` |
| Shared instance master | First `share_instance = yes` process (daemon or `Node`) |
| `~/.reticulum` | `~/.reticulum-go` |

## Concurrency and callbacks

| Component | Rule |
|-----------|------|
| Transport / interfaces | Packet handlers run on interface or transport goroutines |
| Destination / link callbacks | May fire concurrently. Return quickly. Do heavy work in your own goroutine |
| `Link.Request` receipts | Timeout and response callbacks run in separate goroutines |
| Same `Link` | Do not call `Establish`, `Teardown`, and `Request` concurrently without external locking |
| `Node.ReloadInterfaces` / network hooks | Serialized by an internal mutex |
| Identities / destinations | Internally mutex-protected. Still treat callbacks as re-entrant |

Python RNS is largely single-threaded asyncio. Go is multi-threaded by default. Design for that.

## Errors and empty results

| Situation | Typical signal |
|-----------|----------------|
| No path yet | `HasPath` false. Call `RequestPath` and wait |
| Link not ready | `Establish` error or `GetStatus() != StatusActive` |
| Request timeout | `RequestReceipt` status `StatusFailed` |
| Recall before announce | `identity.Recall` error. Wait for announce or seed known destinations |
| Shared instance auth failure | RPC dial / auth error. Align `rpc_key` or transport identity |
| Hardware-bound identity without signer | `ErrHardwareBoundSignerRequired` |

## Other API surfaces

| Surface | Document |
|---------|----------|
| Localhost JSON and WebSocket | [Control API](control-api.md) |
| C ABI (`include/rns.h`) | [librns](librns.md) |
| Browser JS bridge | [Embedding and WebAssembly](embedding-and-wasm.md) |
| CLI tools | [CLI utilities](utilities.md) |
| Crypto details | [Cryptography](cryptography.md) |
| Interface types | [Interfaces](interfaces.md) |

## Related documents

- [Examples](examples.md)
- [Package map](package-map.md)
- [Embedding and WebAssembly](embedding-and-wasm.md)
- [Compatibility](compatibility.md)
- [Python RNS API reference](https://reticulum.network/manual/reference.html) (wire and semantic authority)
