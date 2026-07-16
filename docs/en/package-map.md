# Package map

## Overview

Public API lives under `pkg/`. The daemon and tests import these packages. `internal/` holds daemon-specific wiring that is not a stable import path for external modules.

This page maps each package to its responsibility and primary entry points. For recipes, Python migration, and concurrency rules, see [API reference](api-reference.md).

## Core protocol stack

### `pkg/packet`

Wire packet serialization, header types 1 and 2, hashing, receipts.

| Item | Detail |
|------|--------|
| Key types | `Packet`, `PacketReceipt` |
| Constants | `MTU = 500` |
| Main files | `packet.go`, `receipt.go`, `constants.go` |

### `pkg/identity`

Key generation, recall, sign and verify, encrypt and decrypt, ratchets, optional Secret Service persistence.

| Item | Detail |
|------|--------|
| Key types | `Identity` |
| Hardware signing | `NewIdentityWithSigner`, RHB1 descriptor in `hardware_bound.go` |
| At-rest store | `pkg/identity/store` (`file` or Freedesktop Secret Service), RSSI markers |
| Main files | `identity.go`, `identity_signer.go`, `hardware_bound.go`, `known_persist.go`, `store/` |

### `pkg/securemem`

Best-effort locked buffers for long-term identity private keys (`mlock`, wipe on close).

| Item | Detail |
|------|--------|
| Key types | `Buf` |
| Main files | `buf.go`, `wipe.go`, `alloc_unix.go` |

### `pkg/cryptography`

Single integration point for primitives. See [Cryptography](cryptography.md).

| Item | Detail |
|------|--------|
| Key types | `CryptoProvider`, `Ed25519Signer` |
| Extension | `SetProvider`, `ActiveProvider` |
| Main files | `provider.go`, `stdlib_provider.go`, curve/AES/HKDF helpers |

### `pkg/destination`

Application-facing destinations (SINGLE, GROUP, PLAIN, LINK).

| Item | Detail |
|------|--------|
| Key types | `Destination`, `RequestHandler` |
| Main files | `destination.go`, `constants.go` |

### `pkg/announce`

Announce construction, signing, handler registration.

| Item | Detail |
|------|--------|
| Key types | `Announce`, `Handler` |
| Main files | `announce.go`, `handler.go` |

### `pkg/transport`

Routing engine: path table, announces, forwarding, links, persistence.

| Item | Detail |
|------|--------|
| Key types | `Transport`, `PathInfo` |
| Main files | `transport.go`, `ingress.go`, `relay.go`, `path_selection.go`, `path_persist.go` |

See [Transport](transport.md).

### `pkg/link`

Encrypted bidirectional links, request/response, channel and resource integration.

| Item | Detail |
|------|--------|
| Key types | `Link` |
| Entry points | `HandleIncomingLinkRequest`, `Reestablish`, `WatchAndReconnect` |
| Main files | `link.go`, `link_path_recovery.go`, `reconnect.go` |

### `pkg/pathfinder`

Per-link path lookup table used inside links.

| Item | Detail |
|------|--------|
| Key types | `PathFinder`, `Path` |
| Main file | `pathfinder.go` |

## Data transfer and messaging

### `pkg/channel`

Reliable message delivery over a link.

| Item | Detail |
|------|--------|
| Key types | `Channel`, `Envelope` |
| Main files | `channel.go`, `constants.go` |

### `pkg/buffer`

Stream buffer over channel with bzip2 compression.

| Item | Detail |
|------|--------|
| Key types | `Buffer` |
| Main files | `buffer.go`, `constants.go` |

### `pkg/resource`

Multi-part file transfer, hashmaps, RESOURCE_PRF, bzip2.

| Item | Detail |
|------|--------|
| Key types | `Resource`, `Advertisement` |
| Main files | `resource.go`, `advertisement.go`, `bzip2_compress.go` |

## Network interfaces

### `pkg/interfaces`

All interface implementations and factory.

| Item | Detail |
|------|--------|
| Key types | `Interface`, `BaseInterface` |
| Factory | `NewFromConfigWithContext` in `fromconfig.go` |
| Reconnect | `reconnect.go` |
| Lifecycle | `lifecycle.go` (Enable, Disable, Detach) |
| Go-only QUIC | `quic.go`, `quic_tls.go` (`QUICClientInterface` / `QUICServerInterface`) |

See [Interfaces](interfaces.md).

### `pkg/ifac`

Interface Access Code mask and unmask.

| Item | Detail |
|------|--------|
| Key types | `Identity` (IFAC identity) |
| Main file | `ifac.go` |

### `pkg/i2p`

I2P SAM client, destinations, tunnels.

| Item | Detail |
|------|--------|
| Key types | `Controller`, `Destination`, `Tunnel` |
| Main files | `sam.go`, `controller.go`, `destination.go`, `tunnel.go` |

### `pkg/backbone`

Multiplexed I/O hub for backbone TCP.

| Item | Detail |
|------|--------|
| Key types | `Hub`, `Backend` |
| Backends | auto, epoll, kqueue, io_uring, go |
| Main files | `backbone.go`, `hub.go`, platform pollers |

## Node orchestration and embedding

### `pkg/node`

Embedder API: transport plus interfaces plus lifecycle.

| Item | Detail |
|------|--------|
| Key types | `Node`, `PauseMode`, `LinkReconnectOptions` |
| Lifecycle | `OnNetworkAvailable`, `OnNetworkLost`, `RefreshPaths`, `ReloadInterfaces` |
| Main files | `node.go`, `lifecycle.go`, `reload.go`, `wiring.go`, `netmon.go` |

### `pkg/sharedinstance`

Python `share_instance` equivalent.

| Item | Detail |
|------|--------|
| Key types | `Instance`, `Hooks` |
| Entry | `Attach(cfg, tr, hooks)` |
| Framing | `SendFramed` / `RecvFramed` |
| Main files | `instance.go`, `rpc.go`, `mpconn.go` |

### `pkg/cli`

Subcommand dispatch for the unified `reticulum-go` binary (`Main`, `RunStatus`, `RunID`, `RunProbe`, `RunPath`, `RunCP`, `RunPageserver`).

| Item | Detail |
|------|--------|
| Entry | `Main(opts)` from `cmd/reticulum-go` |
| Docs | [CLI utilities](utilities.md) |

### `pkg/pageserver`

NomadNet-style page and file server used by `reticulum-go pageserver`.

| Item | Detail |
|------|--------|
| Entry | `pageserver.Run` via `cli.RunPageserver` |
| Dynamic pages | `pkg/pageserver/dynamicpage` |
| Sample tree | `examples/pageserver/` |

### `pkg/health`

Node-local mesh integrity and link-health counters. Used by drop-site instrumentation and ops surfaces. Counters never leave the node unless the operator exports them via status RPC or the control API.

| Item | Detail |
|------|--------|
| Key types | `Registry`, `Snapshot`, `Kind` |
| Entry | `health.Inc`, `health.Default` |
| Main files | `registry.go`, `kind.go`, `window.go` |
| Docs | [Security](security.md#local-mesh-health-observe-only), [CLI utilities](utilities.md#rgoslow) |

### `pkg/rnsutil`

Helpers and RPC client for CLI utilities (`reticulum-go status`, `slow`, `id`, `probe`, …).

| Item | Detail |
|------|--------|
| RPC | `DialRPC`, `GetInterfaceStats`, path and link helpers |
| Identity | `.rsg` / `.rsm` / `.rfe` create and verify |
| Probe | `WaitPath`, `SendProbe` |
| Slow report | `AnalyzeSlow`, integrity and bottleneck findings |
| Docs | [CLI utilities](utilities.md) |

### `pkg/wasm`

JavaScript bridge for browser builds (`//go:build js && wasm`).

| Item | Detail |
|------|--------|
| Entry | `RegisterJSFunctions` |
| Main files | `wasm.go`, `lifecycle.go` |

### `pkg/controlapi`

Localhost JSON and WebSocket control plane.

| Item | Detail |
|------|--------|
| Key types | `Server` |
| Main files | `server.go`, `session.go`, `protocol.go`, `ws.go` |

See [Control API](control-api.md).

### `pkg/librns`

C ABI facade for in-process embed. Pure Go core. CGO shims in `pkg/librns/capi`.

| Item | Detail |
|------|--------|
| Header | `include/rns.h` |
| Shared lib | `task build-librns` produces `bin/librns.so` |
| Smoke | `examples/librns-smoke` |
| Odin | `bindings/odin` (`task test-odin`) |
| Main files | `node.go`, `identity.go`, `destination.go`, `link.go`, `queue.go` |

See [librns](librns.md).

### `bindings/odin`

Idiomatic Odin package over `librns.so`. Not a Go import path. Use `-collection:rns=bindings/odin` and `import rns "rns:rns"`.

| Item | Detail |
|------|--------|
| Package | `bindings/odin/rns` |
| Tests | `bindings/odin/tests` |
| Build | `task test-odin` or `make -C bindings/odin test` |
| Platform | Linux (links `system:rns`) |

See [librns](librns.md#odin-bindings).

### `bindings/dart`

Dart package `rns_control` with librns FFI (`ffi.dart`) and a Control API client. Path dependency for Flutter apps.

| Item | Detail |
|------|--------|
| Package | `bindings/dart` (`name: rns_control`) |
| FFI | Linux, Android, Windows via `librns` (`package:rns_control/ffi.dart`) |
| Tests | `dart test` / `task test-dart` |
| Platforms | Flutter mobile and desktop (events need `dart:io`). FFI needs shipped native libs |

See [Control API](control-api.md#dart-and-flutter).

## Discovery and policy

### `pkg/discovery`

rnstransport wire constants, LXStamper, msgpack layouts.

| Item | Detail |
|------|--------|
| Key types | `InterfaceDiscovery` |
| Main files | `discovery.go`, `interface_discovery.go` |

### `pkg/blackhole`

Blackhole table semantics, merge, announce filtering.

| Item | Detail |
|------|--------|
| Key types | `Table`, `Entry` |
| Main file | `blackhole.go` |

### `pkg/rate`

Token-bucket limiter, ingress and egress announce controls.

| Item | Detail |
|------|--------|
| Key types | `Limiter`, `IngressControl` |
| Main file | `rate.go` |

## Configuration and shared types

### `pkg/reticulumconfig`

Canonical INI parser and writer.

| Item | Detail |
|------|--------|
| Functions | `LoadConfig`, `SaveConfig`, `DefaultConfig`, `InitConfig`, `GetConfigPath` |
| Main file | `config.go` |

### `pkg/common`

Shared config structs, path types, IFAC helpers, persistence utilities.

| Item | Detail |
|------|--------|
| Key types | `ReticulumConfig`, `InterfaceConfig`, `Path` |
| Main files | `types.go`, `config.go`, `interfaces.go`, `persistence.go` |

### `pkg/config` (legacy)

Older standalone config struct. New code should use `pkg/reticulumconfig`.

## Security and operations

### `pkg/sandbox`

Post-startup OS restrictions.

| Item | Detail |
|------|--------|
| Entry | `Apply(cfg)` |
| Main files | `sandbox.go`, platform-specific files |

### `pkg/debug`

Structured logging with debug levels 1 through 7.

| Item | Detail |
|------|--------|
| Functions | `Init`, `Log`, `SetDebugLevel` |
| Main file | `debug.go` |

## Utilities

### `pkg/resolver`

Deterministic identity resolution from a full name string (SHA-256).

| Item | Detail |
|------|--------|
| Key type | `Resolver` |
| Main file | `resolver.go` |

## Internal packages (not stable for importers)

### `internal/config`

Re-exports `pkg/reticulumconfig` for the daemon.

### `internal/storage`

Filesystem persistence under `~/.reticulum-go/storage/`.

| Item | Detail |
|------|--------|
| Key type | `Manager` |
| Main files | `manager.go`, `atomic.go` |

## Command binaries

| Path | Binary | Role |
|------|--------|------|
| `cmd/reticulum-go` | `reticulum-go` | Daemon and tools (status, id, probe, path, cp, pageserver). Legacy `rgo*` names are thin wrappers / install symlinks. |
| `cmd/rgostatus` … `cmd/rgocp` | (wrappers) | Call into `pkg/cli` for compatibility with old build scripts |
| `cmd/reticulum-wasm` | WASM module | Browser entry |

CLI dispatch lives in `pkg/cli`. Pageserver logic lives in `pkg/pageserver`.

## Suggested import paths for applications

| Task | Packages |
|------|----------|
| Embed full node | `pkg/node`, `pkg/reticulumconfig`, `pkg/destination`, `pkg/identity` |
| Embed from C / FFI | `pkg/librns` (or link `librns.so` + `include/rns.h`) |
| Embed from Odin | `bindings/odin` (`import rns "rns:rns"`, link `librns.so`) |
| Flutter / Dart FFI | `bindings/dart` (`package:rns_control/ffi.dart`) |
| Flutter / Dart Control API | `bindings/dart` (`package:rns_control`) |
| Low-level transport only | `pkg/transport`, `pkg/interfaces`, `pkg/packet` |
| Crypto only | `pkg/cryptography`, `pkg/identity` |
| Browser | `pkg/wasm` (compiled), WebSocket interface |
| Out-of-process non-Go client | Control API (`pkg/controlapi` on the daemon) |

Do not import `internal/` from outside this module.

## Related documents

- [API reference](api-reference.md)
- [Examples](examples.md)
- [Embedding and WebAssembly](embedding-and-wasm.md)
- [Control API](control-api.md)
- [librns](librns.md)
