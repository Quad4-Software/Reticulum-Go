# Embedding and WebAssembly

| Field | Value |
|-------|-------|
| Document version | 1.1 |
| Last updated | 2026-07-09 |
| Author | Ivan |

## Integration paths

| Path | Use when |
|------|----------|
| `pkg/node` | Go app, full transport and interfaces in-process |
| `pkg/librns` | Native host (C, C++, Qt, Flutter FFI) wants the same stack in-process |
| Control API | Separate language talking to a local `reticulum-go` daemon |
| `pkg/wasm` | Browser client over WebSocket |

## Embedding with pkg/node

`Node` orchestrates transport, interfaces, shared instance, discovery, and lifecycle hooks.

### Minimal sequence

```go
cfg, err := reticulumconfig.LoadConfig(path)
n, err := node.New(cfg)
err = n.Start()

// register destinations with n.Transport()
// start application logic

n.Stop()
```

### Lifecycle hooks

| Method | When to use |
|--------|-------------|
| `OnNetworkAvailable` | NIC came up, Wi-Fi reassociated, or manual resume |
| `OnNetworkLost` | NIC down or airplane mode |
| `RefreshPaths` | Stale paths, force path requests for watched destinations |
| `ReloadInterfaces` | Config file interface blocks changed |
| `StartInterfaceDiscovery` | rnstransport discovery when `discover_interfaces = yes` |

`watch_interfaces = yes` in config enables NIC polling via `netmon.go` on Linux, Android, Windows, macOS, and BSD. WASM builds use a stub.

### Pause modes

`OnNetworkLost` respects `PauseMode`:

- `PauseModeDisable` calls `Disable()` on interfaces (default)
- `PauseModeStop` calls `Stop()` on interfaces

### Link auto-reconnect

`LinkReconnectOptions` and `EnableLinkAutoReconnect` wire `link.WatchAndReconnect` for watched destinations.

### Hot reload

Call `ReloadInterfaces` with updated config or send `SIGHUP` to the daemon on Unix. See [Interfaces](interfaces.md).

### Shared instance

When `share_instance = yes`, `sharedinstance.Attach` runs during `Start()`. If another process already owns interfaces, this process becomes a client and skips local interface binds.

## Direct transport use

Advanced embedders can use `transport.NewTransport` and `interfaces.NewFromConfigWithContext` without `Node`. You must register interfaces, handle shared instance, and wire lifecycle yourself. `Node` is the supported path for most applications.

## WebAssembly build

Binary: `cmd/reticulum-wasm` with build tag `js && wasm`.

Build with Task:

```bash
task build-wasm
task test-wasm
```

### JavaScript API

`pkg/wasm.RegisterJSFunctions` exposes a `reticulum` global:

| Function | Role |
|----------|------|
| `init` | Initialize transport and identity |
| `getIdentity` | Read local identity |
| `getDestination` | Read destination handle |
| `connect` | Connect WebSocket interface |
| `disconnect` | Close WebSocket |
| `isConnected` | Connection state |
| `announce` | Send announce |
| `sendData` / `sendMessage` | Send data packet |
| `requestPath` | Request path to destination |
| `setPacketCallback` | JS callback for packets |
| `setAnnounceCallback` | JS callback for announces |
| `getStats` | Traffic counters |
| `onNetworkAvailable` | Resume after browser online |
| `onNetworkLost` | Pause on offline |
| `setWatchedDestinations` | Watch list for paths |

On load, the module calls the JavaScript function `reticulumReady()` if defined.

### WebSocket interface

WASM uses `WebSocketInterface` to tunnel Reticulum frames to a gateway process or service that owns real UDP/TCP/Auto interfaces. The browser does not open raw UDP multicast.

Architecture:

```
Browser (WASM + JS)
    |
    | WebSocket
    v
Gateway (reticulum-go or custom)
    |
    | UDP / TCP / Auto
    v
Reticulum network
```

### Network lifecycle in the browser

Listen for `online` and `offline` events and call `reticulum.onNetworkAvailable()` or `reticulum.onNetworkLost()` so transport pauses cleanly.

## TinyGo and embedded

The README references a `tinygo` branch for very constrained devices. That branch targets TinyGo 0.41.0 or newer and is separate from the main module build.

## Control API

Run `reticulum-go` with `enable_control_api = yes` and talk HTTP/WebSocket from any language. The daemon owns transport. See [Control API](control-api.md).

## librns C ABI

Thin facade over `pkg/node`, destination, and link. Same wire stack as the daemon, linked into your process.

| Artifact | Role |
|----------|------|
| `include/rns.h` | Public header (`RNS_API_VERSION`) |
| `bin/librns.so` | Shared library (`task build-librns`, needs CGO) |
| `pkg/librns` | Pure Go facade (unit and fuzz without CGO) |

```bash
task build-librns
make -C examples/librns-smoke
./examples/librns-smoke/librns-smoke
```

### librns vs Control API

| librns | Control API |
|--------|-------------|
| In-process | Separate daemon |
| `rns_event_poll` queue | WebSocket events |
| C ABI / FFI | JSON over HTTP and WS |
| Caller-owned buffers | Base64 JSON payloads |

### ABI rules

- Handles are opaque `uint64_t`. Destroy them before exit.
- Events copy into caller buffers. Set `app_data` and `app_data_cap`. Truncation sets `app_data_truncated`.
- The event queue is bounded and drops the oldest entry on overflow.
- `rns_last_error` holds the last failing call message.
- Empty config path uses in-memory defaults with `share_instance` off.

Daemon builds stay `CGO_ENABLED=0`. Only `build-librns` turns CGO on. Linux `.so` first. Other platforms later.

## Sandbox note

OS sandbox (`pkg/sandbox`) applies to the native daemon, not the WASM module. Browser security is enforced by the browser runtime.

## Related documents

- [Architecture](architecture.md)
- [Package map](package-map.md)
- [Examples](examples.md)
- [Getting started](getting-started.md)
- [Control API](control-api.md)
