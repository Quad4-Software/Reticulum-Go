# Changelog

## v1.0.0

First stable release of Reticulum-Go.

Wire compatible with Python RNS 1.3.9

### Included
- Crypto, identity, destinations, packets, transport, links, channels, buffers, and resources
- IFAC on UDP, TCP, Auto, and related interfaces
- Interfaces: UDP, TCP, Auto, I2P, Backbone, Pipe, Local, Serial, Modem73, SDR, WebSocket, QUIC, WebTransport, DNS rendezvous, VSOCK, HTTPS
- Daemon utilities: status, id, probe, path, cp, x, pageserver, slow, speedtest, self-check
- librns C ABI with node lifecycle, control API, and sandbox
- Odin, Zig, C++, Dart, Rust, and Python librns bindings
- Dart librns FFI and Dart Control API client
- Fully ephemeral in-memory storage with soft memory caps and OOM-safe eviction
- librns API version aligned to 1.5
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
