# Changelog

## v1.0.0

First stable release of Reticulum-Go.

Wire compatible with Python RNS 1.3.8

### Included
- Crypto identity destinations packets transport links channels buffers resources
- IFAC on UDP TCP Auto and related interfaces
- Interfaces: UDP TCP Auto I2P Backbone Pipe Local Serial WebSocket QUIC WebTransport DNSRendezvous VSOCK HTTPS
- Daemon utilities: status id probe path cp x pageserver
- librns C ABI node lifecycle control API sandbox
- Odin librns bindings (`bindings/odin`)
- Dart Control API client (`bindings/dart`)
- Fully ephemeral `in_memory_storage` mode with soft memory caps and OOM-safe eviction
- Align `RNS_API_VERSION` in `include/rns.h` with librns `1.1`
- Network identity outgoing flags Go interface plugins
- Split resource advertisements for large transfers
- Logging to stderr file syslog and journald
- Examples: minimal announce link resources filetransfer echo and more
- Go-only underlays: DNS TXT rendezvous, Linux VSOCK, HTTPS long-poll
- Race, fuzz, and goroutine-leak coverage for Serial, WebTransport, DNS rendezvous, VSOCK, and HTTPS
- Channel envelope and StreamDataMessage wire parity with Python RNS
- Live Python-Go interop for channel buffer rncp and blackhole LINKIDENTIFY
- Health drop counters for announce dup, path response suppressions, path request dedup, and link relay unknown iface

### Not in this release
- RNode KISS AX25 Weave radio drivers
- Discovery announcer and autoconnect loops
- Blackhole auto publish federation
- Utilities: rnsh rnir rnpkg rngit
- Remote rnpath rnstransport modes

Hardware radio drivers and remaining utilities are planned for later releases.
