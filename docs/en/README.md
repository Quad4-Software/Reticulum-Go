# Reticulum-Go Documentation (English)

Professional documentation for architects, operators, and engineers working with Reticulum-Go.

| Field | Value |
|-------|-------|
| Document version | 1.0 |
| Last updated | 2026-07-07 |
| Protocol target | Python RNS 1.3.5 |
| Go toolchain | 1.26.4 |
| Author | Ivan |

## About this documentation

These documents describe Reticulum-Go, a Go implementation of the [Reticulum Network Stack](https://reticulum.network/). They are written for readers who need both a system-level view and enough implementation detail to integrate, operate, or extend the stack.

Documentation is organized by language under `docs/`. English lives in `docs/en/`. Additional languages can be added as sibling directories (for example `docs/de/`) without changing the English tree.

## Document index

| Document | Audience | Summary |
|----------|----------|---------|
| [Overview](overview.md) | All | What Reticulum-Go is, goals, feature status, relationship to Python Reticulum |
| [Architecture](architecture.md) | Architects, senior engineers | Layered design, data flow, major components, deployment models |
| [Getting started](getting-started.md) | Operators, developers | Requirements, build, install, first run, basic troubleshooting |
| [Configuration](configuration.md) | Operators | Config file format, paths, keys, interface blocks |
| [Package map](package-map.md) | Engineers | `pkg/` layout, responsibilities, entry points |
| [Transport](transport.md) | Engineers | Routing, path table, announces, forwarding, persistence |
| [Interfaces](interfaces.md) | Operators, engineers | UDP, TCP, Auto, I2P, Backbone, WebSocket, IFAC, reconnect |
| [Identity and destinations](identity-and-destinations.md) | Engineers | Keys, hashes, destination types, announces, requests |
| [Links, channels, and resources](links-channels-and-resources.md) | Engineers | Links, reliable channel, stream buffer, file transfer |
| [Cryptography](cryptography.md) | Security reviewers, engineers | Primitives, key formats, IFAC, ratchets, verification |
| [Security](security.md) | Security, operations | Reporting, sandbox, supply chain, release verification |
| [Compatibility](compatibility.md) | Architects, integrators | Parity with Python RNS, gaps, Go-only extensions |
| [Control API](control-api.md) | Application developers | Localhost JSON and WebSocket API for non-Go clients |
| [Embedding and WebAssembly](embedding-and-wasm.md) | Application developers | `pkg/node` embedder API, WASM build, browser integration |
| [Development and testing](development-and-testing.md) | Developers | Code quality, crossref tests, interop tests, CI |
| [Examples](examples.md) | Application developers | Example programs and how to run them |

## Related files in the repository root

| File | Purpose |
|------|---------|
| [README.md](../../README.md) | Project entry point, quick start, Makefile reference |
| [COMPATIBILITY.md](../../COMPATIBILITY.md) | Detailed compatibility matrix (also summarized in [compatibility.md](compatibility.md)) |
| [SECURITY.md](../../SECURITY.md) | Vulnerability reporting and supply-chain detail (also summarized in [security.md](security.md)) |
| [LICENSE](../../LICENSE) | Apache License 2.0 |

## External references

- [Reticulum manual](https://reticulum.network/manual/reference.html)
- [Reticulum cryptography overview](https://reticulum.network/crypto.html)
- [Python reference implementation](https://github.com/markqvist/Reticulum)
