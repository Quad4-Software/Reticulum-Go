# Changelog

## v1.0.1

### Included
- Rust, Python, Lua, Swift, Java, and Kotlin librns bindings with smoke, page-fetch, and pageserver examples plus CI jobs
- Binding layout and SCAFFOLD updates with examples under each language tree and shared CI runners
- librns ABI 1.5 with identity sign/verify/public-key helpers, RSG surface, and outbound packet send C API
- Link and interface chaos suites wired into task test-chaos alongside transport sim chaos
- Live Go to Python link request and binary packet-burst echo interop
- Expanded test taxonomy covering mutation, health oracles, property suites, CLI smoke and black-box, librns acceptance, UDP path e2e, and matching Task targets
- Docs updates for bindings, librns, examples, and testing layers
- Go-only dos_protection IDS/IPS gates (off/detect/prevent/auto, default auto) with adaptive baselines, once-per-second peak sampling, msgpack-persisted learning, auto promote to prevent, relearn on network or drift change, iface cool-down, crypto and handshake budgets, health counters, rate-limited stdout trip warnings, false-positive edge suites, and live UDP/TCP/mesh protect tests
- Wire compatibility target raised to Python RNS 1.4.0 (discovery stamp default 16, discovery stamp caches, link keepalive parity)

### Fixed
- Link requests register before send so fast replies are not dropped
- Request response and failed callbacks late-fire if attached after completion
- UDP inbound Rx byte and packet counters
- Common base interface IFAC ingress aligned with the shared inbound IFAC policy
- Python, Rust, and Lua node event poll allocate app data so payloads are not silently truncated
- Link watchdog keepalive when remote continuously transmits (RNS 1.4.0)

## v1.0.0

First stable release of Reticulum-Go.

Wire compatible with Python RNS 1.3.9

### Included
- Crypto, identity, destinations, packets, transport, links, channels, buffers, and resources
- IFAC on UDP, TCP, Auto, and related interfaces
- Interfaces: UDP, TCP, Auto, I2P, Backbone, Pipe, Local, Serial, Modem73, SDR, WebSocket, QUIC, WebTransport, DNS rendezvous, VSOCK, HTTPS
- Daemon utilities: status, id, probe, path, cp, x, pageserver, slow, speedtest, self-check
- librns C ABI with node lifecycle, control API, and sandbox
- Odin, Zig, and C++ librns bindings
- Dart librns FFI and Dart Control API client
- Fully ephemeral in-memory storage with soft memory caps and OOM-safe eviction
- librns API version aligned to 1.4
- Persistent identity save for librns hosts
- librns resource transfers and resource events
- Cross-build scripts for Windows DLL and macOS dylib
- Interface discovery announcer for discoverable TCP, Backbone, and I2P peers
- Path and probe reachability diagnostics with status health counters
- Examples: minimal announce, link, resources, file transfer, echo, and more
- librns C, Odin, Zig, and C++ pageserver and page-fetch examples with persistent identities
- Go-only underlays: DNS TXT rendezvous, Linux VSOCK, HTTPS long-poll
- Race, fuzz, and goroutine-leak coverage for Serial, WebTransport, DNS rendezvous, VSOCK, and HTTPS
- Channel envelope and stream data message wire parity with Python RNS
- Live Python-Go interop for channel, buffer, rncp, and blackhole link identify
- Health drop counters for announce duplicates, path response suppressions, path request dedup, and unknown link-relay interfaces
- Shared-instance Unix defaults on Linux matching Python RNS, with TCP fallback when shared instance type is unset

### Not in this release
- RNode, KISS, AX25, and Weave radio drivers
- Discovery autoconnect loops
- Blackhole auto publish federation
- Utilities: rnsh, rnir, rnpkg, rngit
- Remote rnpath and rnstransport modes

RNode and remaining radio drivers plus deferred utilities are planned for later releases.
