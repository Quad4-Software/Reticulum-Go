# Overview

## What is Reticulum-Go

Reticulum-Go is a Go implementation of the [Reticulum Network Stack](https://reticulum.network/). Reticulum is a cryptographic mesh networking protocol designed for resilient communication over heterogeneous links. It can run over UDP, TCP, radio hardware, I2P, and other transports without assuming a single global internet path.

Reticulum-Go targets full wire compatibility with the official Python reference implementation (RNS 1.3.8) while using Go concurrency and static compilation for deployment on servers, desktops, embedded targets, and WebAssembly runtimes.

The primary deliverables are:

- A daemon binary (`reticulum-go`) comparable to Python `rnsd`
- A library surface under `pkg/` for embedding Reticulum in Go applications
- A WebAssembly build (`reticulum-wasm`) for browser clients
- A localhost control API for applications written in other languages

## Design goals

**Protocol interoperability.** Peers running Python Reticulum and Reticulum-Go must exchange packets, establish links, and verify cryptography without translation gateways.

**Portability.** Builds are static by default (`CGO_ENABLED=0`). Cross-compilation targets Linux, Windows, macOS, and WebAssembly. A separate TinyGo branch exists for very small embedded targets.

**Performance and reliability.** Go goroutines handle interface I/O, transport forwarding, and link sessions. Backbone interfaces can use epoll, kqueue, or io_uring multiplexing instead of one goroutine per socket.

**Operational safety.** The daemon applies an OS-level sandbox after startup by default. Dependencies are vendored for reproducible offline builds. Release assets are signed with cosign attestations.

## How Reticulum fits together

Reticulum is not a replacement for IP routing. It is an overlay that gives applications named destinations, encrypted links, and multi-hop paths across whatever physical interfaces you configure.

At a high level:

```
Application  -->  Destination / Link  -->  Transport  -->  Interface  -->  Physical network
```

An application registers interest in a destination hash. Transport learns routes from signed announces and forwards packets hop by hop. Interfaces move bytes on the wire and may apply an Interface Access Code (IFAC) so only authorized peers can join a logical network segment.

See [Architecture](architecture.md) for a fuller picture.

## Feature status

Below is a summary of major features. For line-by-line parity with Python, see [Compatibility](compatibility.md) and [COMPATIBILITY.md](../../COMPATIBILITY.md).

| Area | Status | Location |
|------|--------|----------|
| Wire-compatible crypto and packets | Complete | `pkg/cryptography`, `pkg/packet`, `tests/crossref` |
| Transport core (paths, announces, relay) | Complete | `pkg/transport` |
| Identity and destinations | Complete | `pkg/identity`, `pkg/destination` |
| Links, channel, buffer, resources | Complete | `pkg/link`, `pkg/channel`, `pkg/buffer`, `pkg/resource` |
| IFAC | Complete | `pkg/ifac` |
| UDP, TCP, Auto, I2P, Backbone interfaces | Complete | `pkg/interfaces` |
| WebSocket interface | Go-only | `pkg/interfaces/websocket_*.go` |
| QUIC interface | Go-only | `pkg/interfaces/quic.go`, `quic_tls.go` |
| Daemon and config | Complete | `cmd/reticulum-go`, `pkg/reticulumconfig` |
| Discovery (rnstransport) | Partial | Listening works. Announcer and autoconnect loops are not auto-started |
| Blackhole | Partial | Local drop and LINKIDENTIFY teardown. No publish/federation |
| RNode, KISS, Serial, Weave | Not implemented | No driver in this tree |
| PipeInterface, LocalInterface | Implemented | `pipe.go`, `local.go`, `sharedinstance` |
| Python CLI utilities | Yes (core) | `reticulum-go status|id|probe|path|cp` via `pkg/cli` / `pkg/rnsutil` |
| Interface hot reload | Go-only | `pkg/node/reload.go`, SIGHUP on Unix |
| Control API | Go-only | `pkg/controlapi` |
| librns C ABI | Go-only | `pkg/librns`, `include/rns.h`, `task build-librns` |
| Runtime sandbox | Go-only | `pkg/sandbox` |

## Repository layout

```
Reticulum-Go/
  cmd/
    reticulum-go/       Daemon + tools (status, id, probe, path, cp, pageserver)
    reticulum-wasm/     WebAssembly entry
    librns/             C shared library entry (`-buildmode=c-shared`)
    rgo*/               Thin wrappers for legacy binary names
  include/
    rns.h               Public librns C header
  pkg/                  Public library packages (cli, pageserver, rnsutil, …)
  man/                  Man pages (sections 1 and 8)
  packaging/            nfpm deb/rpm config
  internal/             Daemon-only helpers (config re-export, storage)
  examples/             Sample applications (includes librns-smoke)
  tests/
    crossref/           Byte-level parity with Python vectors
    interop/            Live Go/Python tests
  vendor/               Vendored third-party modules
  docs/
    en/                 English documentation (this tree)
```

## Relationship to Python Reticulum

Python Reticulum (`RNS`) is the reference implementation and defines the on-wire protocol. Reticulum-Go reimplements that protocol in Go and verifies behavior against:

- JSON test vectors generated from the reference tree (`tests/crossref`)
- Live interop tests that run Go and Python side by side (`tests/interop`)

Configuration uses the same INI-style shape as Python (`[reticulum]`, `[logging]`, `[[Interface Name]]`). The default config directory is `~/.reticulum-go` instead of `~/.reticulum` so both stacks can coexist on one machine.

Reticulum-Go adds features that Python does not ship today (control API, librns, sandbox, interface hot reload, NIC watching). Those extensions do not change the wire format unless explicitly documented as Go-only.

## Who should read which document

| Role | Start here |
|------|------------|
| Architect evaluating adoption | This page, then [Architecture](architecture.md) and [Compatibility](compatibility.md) |
| Network operator | [Getting started](getting-started.md), [Configuration](configuration.md), [Interfaces](interfaces.md), [CLI utilities](utilities.md) |
| Go application author | [API reference](api-reference.md), [Package map](package-map.md), [Examples](examples.md), [Embedding and WebAssembly](embedding-and-wasm.md) |
| Native / FFI embedder | [librns](librns.md), [Compatibility](compatibility.md) |
| Security reviewer | [Cryptography](cryptography.md), [Security](security.md) |
| Developer | [Development and testing](development-and-testing.md) |

## License and credit

Reticulum-Go is licensed under Apache License 2.0. See [LICENSE](../../LICENSE).
