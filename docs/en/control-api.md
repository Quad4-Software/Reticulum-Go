# Control API

## Purpose

`pkg/controlapi` exposes a localhost JSON and WebSocket API so applications in any language can use Reticulum destinations, announces, links, and requests without embedding the Go transport stack.

The server is optional and disabled by default.

## Architecture notes

The mesh is destinations, announces, and links between peers. No node is privileged on the wire. Upstream design intent: [Zen of Reticulum](https://reticulum.network/manual/zen.html).

The Control API is a local HTTP/WebSocket front end for one `reticulum-go` process. It is not the mesh. Apps that treat this API as a required remote service reintroduce a single control host even when RNS routing stays peer-to-peer.

**Appropriate uses**

- Tools and UIs on the same host as the daemon
- App logic in another language while the daemon owns transport
- Lab or ops access on loopback (or a private network you fully control)

**Avoid**

- A public Control API endpoint that clients must use to participate
- Putting identity, routing, or app policy behind one always-on control host
- Large transfers via base64 `link.send_resource` when rncp or in-process librns is available
- Binding off loopback and describing the result as decentralized because RNS is underneath

If the product fails when the Control API host is unreachable, the product depends on that host. Prefer peer destinations and links for application traffic. Keep this API on the machine that runs the node.

## Enable in config

```ini
[reticulum]
enable_control_api = yes
rpc_key = <64 hex characters>
control_api_host = 127.0.0.1
control_api_port = 37430
# Optional extra listener. TCP stays enabled.
# control_api_socket = /run/reticulum-go/control.sock
```

rpc_key is a 32-byte value encoded as hex. The same key authenticates shared-instance RPC when configured.

Generate a key with a cryptographic random source. Example using OpenSSL:

```bash
openssl rand -hex 32
```

## Authentication

All `/v1` routes require:

```
Authorization: Bearer <hex rpc_key>
```

Requests without a valid bearer token are rejected.

## HTTP routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/health` | Liveness probe (process up, transport id, uptime). Not mesh integrity scoring |
| GET | `/v1/status` | Interface statistics, including Go local integrity counters when present |
| GET | `/v1/paths` | Path table snapshot |
| POST | `/v1/sessions` | Create session (identity) |
| DELETE | `/v1/sessions/{id}` | Tear down session |
| POST | `/v1/sessions/{id}/destinations` | Register destination |
| POST | `/v1/sessions/{id}/destinations/{hash}/announce` | Send announce |
| POST | `/v1/sessions/{id}/destinations/{hash}/requests` | Bridge request path to WebSocket |
| DELETE | `/v1/sessions/{id}/destinations/{hash}/requests?path=` | Deregister request path |
| POST | `/v1/sessions/{id}/path/request` | Request path to destination. Response includes `wait_s`. Repeats inside 20s return 429 |
| GET | `/v1/sessions/{id}/events` | WebSocket event stream |

Lifecycle routes (Go node integration):

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/lifecycle/resume` | Resume after pause |
| POST | `/v1/lifecycle/pause` | Pause interfaces |
| POST | `/v1/lifecycle/refresh-paths` | Refresh stale paths |

Binary fields (hashes, app data, link payloads) are hex- or base64-encoded as documented in `pkg/controlapi/protocol.go`.

### Status integrity fields (Go daemon)

`GET /v1/status` mirrors shared-instance interface stats. Against a Reticulum-Go daemon each interface object may include:

| JSON field | Meaning |
|------------|---------|
| ifac_fail | IFAC verify failures |
| hmac_fail | Link HMAC failures |
| announce_sig_fail | Invalid announce signatures |
| unpack_fail | Packet unpack failures |
| announce_dup | Duplicate announce ignored |
| path_resp_suppressed | PATH_RESPONSE skipped (next hop is requestor) |
| path_req_dup | Duplicate path request tag ignored |
| path_req_no_cache | Known path without cached announce |
| path_resp_queued_skip | PATH_RESPONSE already queued for iface |
| link_relay_unknown_iface | Link relay dropped unknown source iface |
| integrity_fail_rate | Windowed fails / (fails + accepted) |
| stale_closes | Links closed after going stale |
| link_stale_close | Same lifetime total as exposed on the iface |
| keepalive_timeout | Transitions into keepalive stale |
| clients | Spawned peer count (I2P parent) |
| i2p_connectable | Connectable I2P server tunnel enabled |
| i2p_b32 | Published `*.b32.i2p` endpoint when connectable |
| tunnelstate | I2P peer tunnel label (`Creating Tunnel`, `Tunnel Active`, `Tunnel Unresponsive`) |
| i2p_last_error | Last SAM dial or stream error text for an I2P peer |

These counters are local observability only. They do not change packet accept or reject policy. For scored findings use `reticulum-go slow`. For a full path and health dump use `reticulum-go snapshot`. See [Security](security.md#local-mesh-health-observe-only), [packet-debug](packet-debug.md), and [CLI utilities](utilities.md#rgoslow).

## Sessions

A session owns:

- One identity
- Destinations registered under that identity
- Links opened or accepted by the session

Typical flow:

```
POST /v1/sessions
  -> session id

POST /v1/sessions/{id}/destinations
  -> register app destination

POST /v1/sessions/{id}/destinations/{hash}/announce
  -> publish announce

GET /v1/sessions/{id}/events (WebSocket)
  -> subscribe to announces, links, requests
```

## WebSocket events

Server to client JSON event type values:

| Event | Meaning |
|-------|---------|
| announce | Remote announce received |
| `link.established` | Link is active |
| `link.failed` | Outbound link failed |
| `link.data` | Data received on link |
| `link.closed` | Link closed |
| `link.remote_identified` | Peer identified on link |
| `request.incoming` | Request arrived on registered path |
| `request.response` | Outbound `link.request` succeeded |
| `request.failed` | Outbound `link.request` failed or timed out |
| `resource.started` | Resource transfer started |
| `resource.concluded` | Resource transfer finished |
| `command.error` | WebSocket command could not be applied |

Client to server command type values:

| Command | Meaning |
|---------|---------|
| subscribe_announces | Subscribe to announces. Empty filter means all. Non-empty filter must be an exact 16-byte dest hash hex |
| `link.open` | Open outbound link |
| `link.send` | Send on link |
| `link.close` | Close link |
| `link.request` | Outbound request on established link |
| `link.send_resource` | Send payload as a link resource (base64). Keep payloads small |
| `link.identify` | Identify session identity on link |
| `request.respond` | Answer a request. Optional filename for NomadNet `[name, bytes]` |

Full type definitions: `pkg/controlapi/protocol.go`.

## Links via API

Register a destination with link acceptance enabled for inbound links.

Outbound: send `link.open` over the events WebSocket. The server waits a bitrate-sized path window, then starts handshake. `link.established` or `link.failed` follows. Do not apply a flat 15 second client timer. The `wait_s` field from `POST .../path/request` is the window to show in UI if you request a path yourself.

Both directions receive `link.established` when ready, then `link.data` for peer data.

Use `link.identify` after the link is active. The peer receives `link.remote_identified`.

## Requests via API

Register a request path with `POST .../destinations/{hash}/requests`. Incoming requests appear as `request.incoming`. Respond with `request.respond` before the handler timeout.

Outbound: after `link.established`, send `link.request`. Completion arrives as `request.response` or `request.failed`.

Handlers block the underlying link goroutine until response or timeout. Keep processing short.

Deregister with `DELETE .../requests?path=/your/path`.

## Resources via API

`link.send_resource` mirrors librns minimal resource send. Expect `resource.started` and `resource.concluded` on the peer. Payloads are base64 over WebSocket, so large files are memory-heavy. Prefer rncp or in-process librns for bulk transfers.

## Scope and caveats

This API is an application contract for destinations, announces, links, requests, identify, and minimal resources. It is not a full mirror of channels, stream buffers, resource cancel/progress, or mesh-admin ops (drop path, blackhole). Those stay on shared-instance RPC and CLI.

Control API `/v1` is independent of librns `RNS_API_VERSION`. Additive JSON fields and new type strings are the compatibility model.

WebSocket event delivery is best-effort. A full client outbox drops events.

See [Architecture notes](#architecture-notes) when designing a product on top of this API.

## Example client

`examples/control-client/client.py` is a Python reference client for the API.

## Security notes

- Default bind is loopback only
- Do not expose the control API to untrusted networks without additional protection
- Treat rpc_key as a secret comparable to an API token
- Binding off loopback for convenience fights the model in [Architecture notes](#architecture-notes)
- See [Security](security.md)

## Implementation files

| File | Role |
|------|------|
| `server.go` | HTTP server and routing |
| `session.go` | Session state |
| `protocol.go` | Request and event types |
| `ws.go` | WebSocket handling |
| `auth.go` | Bearer validation |
| `lifecycle.go` | Lifecycle routes |

Daemon wiring: `cmd/reticulum-go/main.go` starts `controlapi.Server` when enabled.

## Related documents

- [Zen of Reticulum](https://reticulum.network/manual/zen.html) (upstream design intent)
- [API reference](api-reference.md) for Go embedders using destinations and links in-process
- [Configuration](configuration.md)
- [Links, channels, and resources](links-channels-and-resources.md)
- [librns](librns.md) for in-process C ABI
- [librns](librns.md#odin-bindings) for in-process Odin bindings
- [Examples](examples.md)

## Dart and Flutter

Path: `bindings/dart/` (package rns_control).

### Control API client

HTTP and WebSocket client for a local or LAN `reticulum-go` daemon. Import `package:rns_control/rns_control.dart`.

```dart
import 'package:rns_control/rns_control.dart';

final client = ControlClient(rpcKey: rpcKey);
final session = await client.createSession();
final events = client.openEvents(session.sessionId);
events.subscribeAnnounces();
```

Coverage includes health, status (with integrity counters), paths, sessions, destinations, announce, request handlers (register and deregister), lifecycle, outbound requests, resources, identify, and WebSocket commands or events. Authenticated WebSocket upgrades require `dart:io` (Flutter mobile or desktop). Browser clients cannot set the Authorization header on WebSocket.

### In-process FFI

For embedding without a daemon, use `package:rns_control/ffi.dart` over librns on Linux, Android, and Windows. See [librns Dart FFI](librns.md#dart-ffi-bindings).

```bash
task build-librns
task test-dart
# or
make -C bindings/dart test
```

Add to a Flutter app with a path dependency:

```yaml
dependencies:
  rns_control:
    path: ../Reticulum-Go/bindings/dart
```
