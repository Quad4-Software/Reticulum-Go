# Changelog

## v1.1.0 [unreleased] - 2026-08-TBD

Wire compatible with Python RNS 1.4.2

### Added
- reticulum-go zen (rgozen symlink): go-fix-style scanner for path and link footguns in Go and optional Python sources. Supports -fix for safe RequestPath error checks, JSON and plain output, rule listing, and test-file scanning
- Release and local cross builds ship linux-amd64 GOAMD64 v1 and v3 together (unsuffixed linux-amd64 stays v1, never v3 alone). linux/386 is also published as linux-i686. Additional CGO-free targets: linux mips/mipsle/mips64/mips64le/ppc64/ppc64le/s390x, OpenBSD, NetBSD, DragonFly, Solaris, illumos, AIX ppc64, and Android arm64

### Transport and pathing
- Discovery path-request timeout scales from the slowest online outgoing fan-out bitrate instead of a flat 15 second wait
- Path-request emit and slowest-bitrate helpers skip receive-only interfaces
- AwaitPath honors the path-response window when the caller sets no deadline
- Repeated nil-tag path requests inside 20 seconds return ErrPathRequestThrottled instead of silent success. NudgePathRequest no longer bypasses throttling
- Local announce bursts beyond 8 in 10 seconds return ErrDestAnnounceThrottled. Path-response announces are not capped
- Link-relay proof timeout adds outbound-interface MTU airtime on the next hop (extra_link_proof_timeout), not on the receive interface
- Forwarded announces that exceed announce_cap wait on a per-interface outgoing queue instead of being dropped

### Links
- Link establishment per-hop timeout is 6 seconds, matching RNS 1.4.2 Link.ESTABLISHMENT_TIMEOUT_PER_HOP
- Link Request rejects a duplicate in-flight path request and caps pending requests at 8
- Send, Request, and Identify on a non-active link return ErrLinkNotActive with a callback hint
- A second outbound Establish to the same destination while a handshake is pending returns ErrLinkEstablishBusy
- Calling Establish again on the same link returns ErrLinkAlreadySettled or ErrLinkEstablishBusy

### Performance
- Destination name hashing lives in pkg/identity so hash-only tools need not import destination ratchets and msgpack. destination.Hash and HashFromIdentityHash remain wrappers
- Link and transport timeout numbers alias pkg/common so importers can use the values without pulling link.go
- Optional QUIC, WebTransport, I2P, and SDR interface drivers register at init. Default builds still include them. `-tags rns_slim` omits those drivers (quic-go, I2P, SDR) from the binary
- Link SendPacket encrypts into the packed HT1 buffer (one wire allocation) and reuses per-link AES and HMAC state
- AES-CBC encrypt/decrypt no longer allocates cipher.NewCBCEncrypter per packet
- Packet receipts use a single AfterFunc timer instead of a goroutine plus 1s ticker
- Default loglevel 4 is info (was verbose), matching Python RNS. Per-packet and handshake traces moved to verbose/trace/packets
- debug.Log returns after an atomic level check with no extra slice or mutex on the filtered path
- Hot-path debug.Log call sites skip argument slices when the level is filtered
- HandlePacket workers start at GOMAXPROCS (floor 4) and grow toward max_packet_handlers only when the ingress queue is full, instead of spawning 512 idle goroutines
- reticulum-go zen scans with go/parser instead of golang.org/x/tools/go/packages so the daemon binary no longer links the go/packages toolchain

### Control API and librns
- path/request responses include wait_s. link.open waits for AwaitPath before handshake
- librns LinkOpen waits for AwaitPath before handshake
- Control API path-request repeats return HTTP 429 with wait_s
- Interface stats `type` is the concrete driver name (UDPInterface, TCPClientInterface) matching Python class names
- Per-interface outgoing announce queue (`announce_queue`) and `drop_announce_queues` matching Python announce_cap delay

### Fixed
- Shared-instance local clients still receive path and link relay when enable_transport is disabled (Python from_local_client and for_local_client_link). PATHREQUEST was already forwarded, but LINKREQUEST and link data were dropped, so rngit and other Python apps resolved a path then failed to establish a link
- Path and link relay developer errors: no-path link relay, transport-disabled relay, no outgoing interface for path request, and interface-not-ready path request now return explicit errors instead of silent success or misleading no-destination logs
- AwaitPath timeout returns ErrNoPathToDestination with destination hash and hint instead of bare context.DeadlineExceeded
- BaseInterface bandwidth stats now increment TxPackets so transmitted-byte and packet counters stay aligned
- CI bench-gate no longer hangs in transport.test. sim Close waited on handler-pool Sends blocked on full inboxes
- CI fuzz-guided skips package unit tests and uses short coverage so the job fits the 45 minute limit
- Channel retry timeout matches Python `_get_packet_timeout_time` (max(rtt*2.5, 0.025) and tx-ring + 1.5)
- Channel start window is 1 when link RTT is above RTT_SLOW, matching Python Channel
- dos_protection defaults to off. core_router no longer forces prevent. Iface-wide cool-down is opt-in so a busy public UDP listener is not blackholed. Path-request and data class ride prefer-keep leniency so discovery survives announce floods
- `drop_announce_queues` RPC cleared the path announce cache instead of per-interface outgoing announce queues

### Tests
- Golden Python RNS 1.4.2 wire vectors and oracles for packet flags/contexts/MDU, announce payload and destination hashes, channel envelopes and RTT windows, resource advertisements, link MDU and establishment timeouts, and adaptive path-request windows (5 bit/s floor, receive-only skipped, discovery timeout vs 15s)

### Docs
- Configuration, API reference, control API, transport, links, utilities, development-and-testing, compatibility, and package-map docs updated for throttling, timeouts, relay behavior, reticulum-go zen, and log levels


## v1.0.2 - 2026-08-14

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

## v1.0.1 - 2026-07-25

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

## v1.0.0 - 2026-07-19

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
