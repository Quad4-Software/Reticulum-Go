# Control API

| Field | Value |
|-------|-------|
| Document version | 1.0 |
| Last updated | 2026-07-07 |
| Author | Ivan |

## Purpose

`pkg/controlapi` exposes a localhost JSON and WebSocket API so applications in any language can use Reticulum destinations, announces, links, and requests without embedding the Go transport stack.

The server is optional and disabled by default.

## Enable in config

```ini
[reticulum]
enable_control_api = yes
rpc_key = <64 hex characters>
control_api_host = 127.0.0.1
control_api_port = 37430
```

`rpc_key` is a 32-byte value encoded as hex. The same key authenticates shared-instance RPC when configured.

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
| GET | `/v1/health` | Node health |
| GET | `/v1/status` | Interface statistics |
| GET | `/v1/paths` | Path table snapshot |
| POST | `/v1/sessions` | Create session (identity) |
| DELETE | `/v1/sessions/{id}` | Tear down session |
| POST | `/v1/sessions/{id}/destinations` | Register destination |
| POST | `/v1/sessions/{id}/destinations/{hash}/announce` | Send announce |
| POST | `/v1/sessions/{id}/destinations/{hash}/requests` | Bridge request path to WebSocket |
| POST | `/v1/sessions/{id}/path/request` | Request path to destination |
| GET | `/v1/sessions/{id}/events` | WebSocket event stream |

Lifecycle routes (Go node integration):

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/lifecycle/resume` | Resume after pause |
| POST | `/v1/lifecycle/pause` | Pause interfaces |
| POST | `/v1/lifecycle/refresh-paths` | Refresh stale paths |

Binary fields (hashes, app data, link payloads) are hex- or base64-encoded as documented in `pkg/controlapi/protocol.go`.

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

Server to client JSON events include:

| Event | Meaning |
|-------|---------|
| `announceEvent` | Remote announce received |
| `linkEstablishedEvent` | Link is active |
| `linkFailedEvent` | Outbound link failed |
| `linkDataEvent` | Data received on link |
| `linkClosedEvent` | Link closed |
| `requestIncomingEvent` | Request arrived on registered path |

Client to server commands include:

| Command | Meaning |
|---------|---------|
| `subscribeAnnouncesCommand` | Filter announces |
| `linkOpenCommand` | Open outbound link |
| `linkSendCommand` | Send on link |
| `linkCloseCommand` | Close link |
| `requestRespondCommand` | Answer a request |

Full type definitions: `pkg/controlapi/protocol.go`.

## Links via API

Register a destination with link acceptance enabled for inbound links.

Outbound: send `linkOpenCommand` over the events WebSocket after the path exists (from announce or path request).

Both directions receive `linkEstablishedEvent` when ready, then `linkDataEvent` for peer data.

## Requests via API

Register a request path with `POST .../destinations/{hash}/requests`. Incoming requests appear as `requestIncomingEvent`. Respond with `requestRespondCommand` before the handler timeout.

Handlers block the underlying link goroutine until response or timeout. Keep processing short.

## Example client

`examples/control-client/client.py` is a Python reference client for the API.

## Security notes

- Default bind is loopback only
- Do not expose the control API to untrusted networks without additional protection
- Treat `rpc_key` as a secret comparable to an API token
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

- [Configuration](configuration.md)
- [Links, channels, and resources](links-channels-and-resources.md)
- [librns](librns.md) for in-process C ABI
- [Examples](examples.md)
