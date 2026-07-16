# Changelog

## Unreleased

- Odin bindings for librns under `bindings/odin` with Task, Makefile, and CI coverage
- Align `RNS_API_VERSION` in `include/rns.h` with librns `1.1`
- Document Odin bindings across README feature list, embedding paths, and compatibility tables

## v1.0.0

First stable release of Reticulum-Go.

Wire compatible with Python RNS 1.3.8 for the software transport stack.

### Included
- Crypto identity destinations packets transport links channels buffers resources
- IFAC on UDP TCP Auto and related interfaces
- Interfaces: UDP TCP Auto I2P Backbone Pipe Local Serial WebSocket QUIC WebTransport DNSRendezvous VSOCK HTTPS
- Daemon utilities: status id probe path cp x pageserver
- librns C ABI node lifecycle control API sandbox
- Odin librns bindings (`bindings/odin`)
- Network identity outgoing flags Go interface plugins
- Split resource advertisements for large transfers
- Logging to stderr file syslog and journald
- Examples: minimal announce link resources filetransfer echo and more
- Go-only underlays: DNS TXT rendezvous, Linux VSOCK, HTTPS long-poll
- Race, fuzz, and goroutine-leak coverage for Serial, WebTransport, DNS rendezvous, VSOCK, and HTTPS

### Not in this release
- RNode KISS AX25 Weave radio drivers
- Discovery announcer and autoconnect loops
- Blackhole auto publish federation
- Utilities: rnsh rnir rnpkg rngit
- Remote rnpath rnstransport modes

Hardware radio drivers and remaining utilities are planned for later releases.
