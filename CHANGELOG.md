# Changelog

## v1.0.3 [unreleased]

### Included
- Transport discovery path-request timeout sized from slowest online outgoing fan-out bitrate instead of a flat 15s
- Link-relay proof timeout adds outbound-interface MTU airtime (`extra_link_proof_timeout` on the next hop, not the receive iface)
- `SlowestOnlineBitrate` and path-request emit skip receive-only interfaces
- `Transport.AwaitPath` waits `PathResponseWindow` when the caller has no deadline
- Control API `path/request` returns `wait_s` and `link.open` waits `AwaitPath` before handshake
- librns `LinkOpen` waits `AwaitPath` before handshake
- Nil-tag `RequestPath` repeats inside 20s return `ErrPathRequestThrottled` instead of silent success (`NudgePathRequest` no longer bypasses)
- Local `Destination.Announce` bursts over 8 in 10s return `ErrDestAnnounceThrottled` (path-response announces are not capped)
- Link `Request` rejects a duplicate in-flight path and caps pending requests at 8
- `Send` / `Request` / `Identify` on a non-active link return `ErrLinkNotActive` with a callback hint
- A second outbound `Establish` to the same destination while a handshake is still pending returns `ErrLinkEstablishBusy`
- Calling `Establish` again on the same link returns `ErrLinkAlreadySettled` or `ErrLinkEstablishBusy`
- Control API path-request repeats return HTTP 429 with `wait_s`

### Fixed
- Shared-instance local clients still get path and link relay when `enable_transport = no` (Python `from_local_client` / `for_local_client_link`). PATHREQUEST already forwarded, but LINKREQUEST and link data were dropped, so rngit and other Python apps resolved a path then failed to establish a link.
- `BaseInterface.updateBandwidthStats` increments `TxPackets` so interface TX packet counters match transmitted bytes
- CI bench-gate no longer hangs in `transport.test` (sim Close waited on handler-pool Sends blocked on full inboxes)
- CI fuzz-guided skips package unit tests and uses `-short` coverage so the job fits the 45m limit

## v1.0.2

Wire compatible with Python RNS 1.4.2

### Included
- Wire compatibility target raised to Python RNS 1.4.2
- Path-request emit skips offline interfaces (and re-checks at recursive PR emit time)
- Adaptive path-request and link-establishment waits from slowest online bitrate (5 bit/s floor) instead of a flat 15s
- Config `bitrate` applied when interfaces are created so adaptive timeout math sees radio timing
- `Transport.FirstHopTimeout` matches Python next-hop airtime, including RPC `get_first_hop_timeout`
- `Link.EstablishmentTimeout` and `rnsutil` FirstHop/PathResponse helpers wired through CLI utilities (Windows shared-instance RPC included)
- Discovery drops blackholed transport ids and announcer identities at announce receive time
- Blackhole `ActiveIdentitySet` for bulk membership checks used by discovery filtering
- Go-unique path-request readiness also refuses non-positive bitrate when exposed (uninitialized radio timing)
- Go-unique blackhole active-identity set invalidates on mutation instead of a fixed 60s TTL
- Go-unique discovery blackhole filtering at receive time (fail-closed) rather than list-only
- Destination identity ratchets on SINGLE announce, remember, and encrypt (`EnableRatchets` / `EnableRatchetsInMemory` / `EnforceRatchets`)
- Known-peer ratchet public keys stored by destination hash in Python-compatible `{ratchet, received}` msgpack
- Pageserver ratchet private-key path uses `{destination_hash}`, matching Python LXMF
- librns `rns_destination_enable_ratchets` plus Destination ratchet helpers in C, Odin, Zig, C++, Dart, Rust, Python, Lua, Swift, Java, and Kotlin bindings
- GROUP destinations with Token PSK (`CreateKeys` / `LoadPrivateKey`), AES-256 Token encrypt/decrypt, local broadcast, and one-hop drop
- Live Go to Python ratchet and GROUP packet interop
- Incoming link handshake slots count against `MaxRegisteredLinks` until register or reject
- Tunnel table drops expired rows on insert and caps live tunnels at 256
- Channel `IsReadyToSend` / `WaitReady` / `MDU` and RTT-class window grow/shrink matching Python Channel
- `reticulum-go sh` / rgosh interactive remote shell (native protocol, automatic rnsh dest detection, `--compat` to force Python rnsh)
- rgosh long-lived PTY sessions, Ctrl-C forwarding, `~.` / `~L` / `~?` escapes, `-A`, `-C` reject, `-b PERIOD` announce (default 900s), Windows pipe fallback
- Remote `rgopath` / `rgostatus` over `rnstransport.remote.management` (`-R` / `-i`, config `enable_remote_management`)
- rgosh TTY handling via golang.org/x/term, Unix vs non-Unix signal files, and adversarial protocol corpus tests
- dos_protection snapshot on status JSON, Control API, and rgoslow findings
- dos_max_* config knobs, bitrate-scaled adaptive floors, priority ingress shedding
- Per-peer fair-share admit buckets on shared UDP/TCP/QUIC/VSOCK/HTTPS/I2P listeners so one sender cannot cool down the whole iface
- Handler pool overflow always sheds (no sync dispatch on ingress threads)
- Fixed HandlePacket worker pool (`max_packet_handlers`, default 512) instead of a goroutine per packet
- 64 KiB stream reads on TCP/QUIC/VSOCK/WebTransport/I2P/Local/Pipe/backbone with packet-MTU HDLC framing unchanged
- Announce ingest at default Info matches Critical (~5 allocs, ~50 µs) after demoting per-packet success logs
- Known destinations stored as structs in RAM (hex msgpack keys only on disk), Identity reused on re-announce
- Link encrypt uses one result buffer (~9 allocs). Backbone HDLC assembler idle cap is 64 KiB at 1 MiB iface MTU
- `node_profile` overlay (`core_router` / `embedded`) fills unset knobs only
- HDLC burst and Unpack hop-gate live Go/Python oracles
- Backbone package mutation testing in CI
- FreeBSD sandbox SIGHUP re-exec for config reload under CapEnter
- Linux Landlock via `github.com/landlock-lsm/go-landlock` instead of hand-rolled syscalls
- Sandbox Landlock/seccomp soft-fail stdout warnings
- Sandbox extra path allowlisting from interface Device, pipe/discovery commands, TLS files, and `sandbox_extra_paths`
- Opt-in `sandbox_strict`, `sandbox_profile=router`, `sandbox_exec_rlimits`, `sandbox_skip_scoped`, and Control API `control_api_socket`
- systemd ProtectSystem and related hardening, plus optional User= drop-in example
- Path jail with symlink resolution for pageserver and `rgocp` listen fetch
- Crypto, IFAC, announce-auth, ratchet-downgrade, and RESOURCE_HMU oracle tests
- Dependabot for GitHub Actions, PR dependency review, and CI RNS pin raised to 1.4.2
- Builtin protocols vendor module renamed from reticulum-go-mf to reticulum-go-protocols
- nfpm deb/rpm/arch packages with tool symlinks, man pages, post-install systemd hook, and Makefile `stage-nfpm`
- Windows XP and Server 2003 release builds via go-legacy-winxp
- Tagged releases ship 386 binaries for Linux, Windows, and FreeBSD (arm v6 already included)
- Tagged releases ship riscv64 binaries for Linux and FreeBSD
- Linux ppc64 serial special-baudrate support
- DragonFly BSD TCP keepalive socket options
- Tree `.rsm` inventory generator and verify/sign script updates
- Docs updates for ratchets, GROUP Token keys, rgosh, sandbox, and dos_protection

### Fixed
- rgosh no longer kills the remote process after 5 seconds, so interactive shells stay up
- Recursive path-request fan-out no longer targets ifaces that went offline while the discovery queue drained
- RESOURCE_HMU negative hashmap segment no longer panics the process
- Tunnel expiry used a bare integer (nanoseconds) instead of 8 hours
- Channel packet timeouts no longer fire while the mutex is dropped, and delivered callbacks wait for link proof instead of running immediately
- Channel inbound envelopes are delivered in sequence order with duplicates dropped, matching Python Channel._receive
- Channel.Send refuses a full TX window and packed envelopes larger than the outlet MDU, matching Python Channel.send
- RESOURCE_HMU hashmap segment indexes that overflow integer multiply no longer wrap into earlier slots
- `Identity.GetCurrentRatchetKey` no longer auto-generates keys (on-wire SINGLE ratchets live on Destination)
- Local `Destination.Announce` requires IN, skips access-point ifaces (Python outbound mode rules), and `Identity.Remember` rejects dest-hash public-key collisions
- Link identify callback mutex so concurrent identification packets do not race remote identity assignment

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
- Wire compatibility target raised to Python RNS 1.4.1 (interface gravity, LRPROOF path rebalance, announces_to_internal, boundary search modes, request size caps)
- Go-unique pathingAffinity (live iface penalty on gravity contests) and LRPROOF rebalance dampening / gravity-sticky hop-increase refusal
- Backbone `blocked_ips` / `blocked_ip_list` in interface stats, status, and control API
- Background known-destination cleaning with path/age rules and cooperative yields
- Initiator keepalive throttle on `lastKeepaliveNs` so one-way data traffic does not suppress probes
- Destination `SetMaxRequestSize` and Link `RequestLimited` (`max_response_size`)

### Fixed
- Link requests register before send so fast replies are not dropped
- Request response and failed callbacks late-fire if attached after completion
- UDP inbound Rx byte and packet counters
- Common base interface IFAC ingress aligned with the shared inbound IFAC policy
- Python, Rust, and Lua node event poll allocate app data so payloads are not silently truncated
- Link watchdog keepalive when remote continuously transmits (RNS 1.4.0)
- Backbone client count and blocked-IP fields missing from RPC interface stats

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
- Utilities: Python rnsh (use rgosh), rnir, rnpkg, rngit
- Remote rnpath and rnstransport modes

RNode and remaining radio drivers plus deferred utilities are planned for later releases.
