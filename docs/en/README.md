# Reticulum-Go Documentation (English)

System documentation, API references, and configuration guides for Reticulum-Go.

## About this documentation

These documents describe Reticulum-Go, a Go implementation of the [Reticulum Network Stack](https://reticulum.network/). They are written for readers who need both a system-level view and enough implementation detail to integrate, operate, or extend the stack.

Documentation is organized by language under `docs/`. English lives in `docs/en/`. Additional languages can be added as sibling directories (for example `docs/de/`) without changing the English tree.

## Document index

| Document | Summary |
|----------|---------|
| [Overview](overview.md) | What Reticulum-Go is, goals, feature status, relationship to Python Reticulum |
| [Architecture](architecture.md) | Layered design, data flow, major components, deployment models |
| [Getting started](getting-started.md) | Requirements, build, install, first run, basic troubleshooting |
| [Configuration](configuration.md) | Config file format, paths, keys, interface blocks |
| [Package map](package-map.md) | `pkg/` layout, responsibilities, entry points |
| [API reference](api-reference.md) | Task-first Go API: recipes, types, Python map, concurrency |
| [Transport](transport.md) | Routing, path table, announces, forwarding, persistence |
| [Interfaces](interfaces.md) | UDP, TCP, Auto, I2P, Backbone, Serial, WebSocket, QUIC, WebTransport, DNS rendezvous, VSOCK, HTTPS, IFAC, reconnect |
| [Identity and destinations](identity-and-destinations.md) | Keys, hashes, destination types, announces, requests |
| [Links, channels, and resources](links-channels-and-resources.md) | Links, reliable channel, stream buffer, file transfer |
| [Cryptography](cryptography.md) | Primitives, key formats, IFAC, ratchets, verification |
| [Security](security.md) | Reporting, sandbox, supply chain, release verification |
| [Compatibility](compatibility.md) | Parity with Python RNS, gaps, Go-only extensions |
| [Control API](control-api.md) | Localhost JSON and WebSocket API for non-Go clients, Dart and Flutter |
| [librns](librns.md) | C ABI map, supported surface, events, build and smoke, Odin bindings |
| [CLI utilities](utilities.md) | `reticulum-go` subcommands (status, id, probe, path, cp, pageserver), shared-instance RPC with Python |
| [Embedding and WebAssembly](embedding-and-wasm.md) | `pkg/node`, WASM, browser integration |
| [Development and testing](development-and-testing.md) | Code quality, crossref tests, interop tests, CI |
| [Examples](examples.md) | Example programs and how to run them |

## Related files in the repository root

| File | Purpose |
|------|---------|
| [README.md](../../README.md) | Project entry point, quick start, Makefile reference |
| [COMPATIBILITY.md](../../COMPATIBILITY.md) | Detailed compatibility matrix (also summarized in [compatibility.md](compatibility.md)) |
| [SECURITY.md](../../SECURITY.md) | Vulnerability reporting and supply-chain detail (also summarized in [security.md](security.md)) |
| [LICENSE](../../LICENSE) | Apache License 2.0 |

## External references

- [Reticulum manual](https://reticulum.network/manual/reference.html) (Python API and protocol authority)
- [Reticulum cryptography overview](https://reticulum.network/crypto.html)
- [Python reference implementation](https://github.com/markqvist/Reticulum)

Go application authors should start with [API reference](api-reference.md) and [Examples](examples.md), then use the Python manual when comparing wire semantics.
