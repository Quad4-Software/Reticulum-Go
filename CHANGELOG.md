# Changelog

## Unreleased

### Changed

- The contributor license grant is replaced by plain DCO sign-off. The `Signed-off-by:` trailer now certifies only the Developer Certificate of Origin.
- The librns event queue now protects payload-bearing events under overflow: non-priority events such as announces are dropped first, and capacity grew from 256 to 4096.
- Mutation testing is now a per-package CI matrix, cutting the job's wall time from roughly 50 minutes to the slowest single package.
- The amd64 test run now builds with `-vet=all` and uploads a `coverage.out` artifact.

### Added

- librns `rns_node_reload_config` hot-reloads interface blocks from the create-time config path without restarting transport. The C ABI is now 1.6, and the Java binding gains `Node.reloadConfig` plus `packetSend` and `destinationEncrypt` helpers.
- `buffer.WriterOptions` with a `CompressionPolicy`: `CompressionAuto` keeps the Python-compatible compression probes, and `CompressionDisabled` always emits the standard uncompressed stream message. New constructors: `NewRawChannelWriterWithOptions`, `CreateWriterWithOptions`, `CreateBidirectionalBufferWithOptions`. `RawChannelWriter.WriteContext` adds caller-controlled cancellation of the TX-window wait. Wire format is unchanged and Python receivers accept both forms.

- `testsummary` honours `TESTSUMMARY_RERUN_FAILS=N`: failed tests get up to N fresh `-count=1` retries, and tests that pass on retry are reported as FLAKE instead of failing the run.
- A `flake-report` workflow files or updates `ci-flake` issues for tests that fail or only pass on retry, and a nightly `stress` workflow repeats the timing-sensitive tests so flakes surface off the merge path.
- All pinned CI toolchain installers (`setup-go`, `setup-node`, `setup-task`, `setup-zig`, `setup-odin`, `setup-kotlin`, `setup-tinygo`, `setup-swift`) now verify the resolved binary reports the pinned version and fail loudly on shadowing.
- A `merge-to-master` dispatch workflow merges dev into master only when the latest CI run for the dev head is green.
- Optional RNE1 passphrase-encrypted identity files. Argon2id plus XChaCha20-Poly1305 wrap the standard 64-byte blob, unlocked by prompt, RETICULUM_IDENTITY_PASSPHRASE, a passphrase fd, or an OS wrap store (Linux kernel keyring and Secret Service, macOS Keychain, Windows DPAPI). rgoid gains -to-passphrase, -to-wrapped, -rekey, and -to-file decryption. Local storage format only, no wire change, and decryption always restores the standard file.

### Security

- A failed unlock of an encrypted transport identity no longer falls through to writing a fresh plaintext identity over the file. Startup fails with the real error instead.

### Fixed

- rgosh listener sessions could deny an Exec that arrived while the version reply was still in flight, tearing down valid connections. The listener now enters WAIT_CMD before the reply is sent.
- Channel inbound dispatch is serialized per channel, so parallel transport workers can no longer deliver stream or channel messages out of sequence order.
- `Channel.WaitReady` and `WaitTxIdle` now wake on TX-ring removal, window changes, and link teardown instead of polling every 5 ms, which removed a roughly 4 MiB/s throughput ceiling on buffered streams.
- A Backbone stream could lose its write interest when a queued send raced the empty-buffer disarm, stranding queued bytes. The interest decision is now made under the stream lock.
- `RawChannelWriter` derives the stream payload bound from the live channel MDU like Python, instead of a fixed 457 bytes, which could produce envelopes larger than a negotiated link MDU.

## v1.3.0 - 2026-09-19

The license is now the Reticulum License, matching Python RNS. [LEGAL.md](LEGAL.md) records the boundary commits.

### Added

- rngit NomadNet pages on a dedicated nomadnetwork.node destination. Same page set as Python rngit 1.5.2, plus file download.
- Templates in the config directory can override those pages. Executable templates hit a timeout and an output cap.
- READMEs and work documents render from Markdown into Micron. Blob pages can syntax-highlight.
- Optional repo stats (views, fetches, pushes, downloads) with charts, and a list of identities whose pushes do not count.
- Thanks counters on repo and release pages. One thanks per link.
- Commits need a Signed-off-by line. The commit-msg hook checks, and so does CI. `SKIP_DCO_HOOK=1` skips the hook.
- Release attestations carry an RFC 3161 timestamp. `COSIGN_REKOR_URL` also uploads them to a transparency log.
- `openvex.json` tells Trivy which scanner findings do not apply here.
- oss-fuzz files for the wire-parser targets.
- `task gitsign:setup` for keyless commit signing, including a private Sigstore stack. `task gittuf:init` scaffolds a gittuf policy.
- Live interop against Python RNS 1.5.4: probes both ways, rncp into rgocp, buffer streams, a full TCP session, serial, pipe, WebSocket, WebTransport, speedtest, and snapshot. CI runs that suite on ubuntu-latest.

### Security

- A 4-byte msgpack map with an unhashable key crashed the process, including on rnsgit requests. Decode rejects those keys. Bin keys become strings, as in Python. Upstream fix is msgpack v5.9.2.
- One panicking packet no longer takes the daemon with it.
- Split-resource assembly rejects replays, gaps, and inconsistent segment counts. Four assemblies per link, a global cap on trackers, and nothing over 1 GiB.
- Advertisement field `d` is the whole resource size on every segment, matching Python. It used to be the size of that segment alone.
- Compressed segments will not expand past `min(d, AutoCompressMaxSize)`. Oversized single-part ads are refused up front.
- A channel stops buffering once 8 MiB is sitting unread. bzip2 streams are capped the same way.
- Channels drop anything more than 48 messages ahead of the next one they expect.
- Each RNode direction keeps 256 packets, then drops the newest.
- `AcceptsLinks` set to false now actually refuses the request. The default is still true.
- `POST /v1/sessions` took an `identity_path` and would read or create keys anywhere on the host. Paths are confined to the server's own identity directory now, symlinks included.

### Removed

- GPU stamping. Stamps stay on the CPU and stay byte-identical. purego is gone, so Landlock can run with CGO off.
- Helpers nothing called anymore, including the old GPU sha256 oracle.

### Fixed

- Valid packets died on IFAC interfaces. The hop check read the still-masked header and treated mask bytes as the hop count. The check now runs after the mask comes off.
- Python UDP peers in the live tests never sent anything. The configs wrote `target_host`. Python reads `forward_ip`. Go still accepts both.
- Two copies of the same announce could both be forwarded. The "seen" slot is claimed when the announce is first noticed.
- `dos_protection` auto mode would not arm if another interface kept making noise. Quiet streaks are counted per interface.
- UDP threw away bursts whenever the reader was busy. Send and receive buffers had been pinned near 1 KiB. They use the kernel default now.
- linux/ppc64 did not build. The serial package only had a special-baud stub for ppc64le. ppc64 has the same stub, and vendor-sync puts it back after a re-vendor.

### Changed

- pbt 1.0.2 brings shrinkers with the generators, so tests dropped the extra shrinker wiring.
- Test names match the technique: malformed, edge, oracle, fault, perf, golden, fuzz.
- rngit pages moved onto nomadnetwork.node, which is where NomadNet goes looking. Serving blobs at `/media` is still a Go-only extra.

### Tests

- The dos-protect flood uses announce frames, the traffic that gate is supposed to shed. All-zero frames were sailing through on purpose.
- The NomadNet crawl walks the public peer list. The old single uplink accepts TCP and forwards nothing.
- The rgosh compat test holds stdin open. Python rnsh 1.5.x loses the remote exit code if stdin hits EOF while the child is still running. Python-to-Python rnsh hits the same race.

## v1.2.0 - 2026-09-15

RNode radios, rngit releases, and the same short flags as the Python 1.5.2 tools.

### Added

- `rgostatus -p` for packets per second, `-m`/`-I` to watch, `-z` to ask for a profile.
- `rgopath -p` fetches the published blackhole list.
- Interface stats include receive and transmit packets per second.
- rngit releases, in the RNS 1.5.x flow: create, list, view, fetch, delete, latest.
- Identity aliases, a block list, and a check that reserved targets stay reserved.
- `/media` blob serving, with optional WebP conversion.
- A template for remote requests that never identified.
- A live profiler, with a shared-instance RPC and the same data on remote `/status`.
- RNode, matching RNS 1.5.2: KISS, firmware gate, radio config, flow control, beacons, PHY stats. Serial and `tcp://` on Linux (including Android), macOS, Windows, FreeBSD, and OpenBSD.
- RNodeMultiInterface: virtual ports, `SEL_INT`, nested sub-interfaces, shared firmware and flow control.
- RNode config reload checks, IFAC defaults, and an in-memory radio you can fuzz.
- Android USB host, BLE Nordic UART, and classic RFCOMM, wired up as `usb://`, `ble://`, and `bt://`.

### Changed

- Short flags match the Python 1.5.2 tools. `rgopath` gained the blackhole and queue-drop flags. `rgocp -f` takes the remote path as a positional.
- Module path is `github.com/Quad4-Software/Reticulum-Go`. First-party deps are real module versions, and `go.mod` has no `replace` lines (#13).
- Commits after v1.1.1 were re-authored to ivan@quad4.io and signed with GPG instead of Reticulum signatures.

### Fixed

- The daemon panicked in Landlock when CGO was off, because the GPU stack pulled in fakecgo. GPU support is opt-in (`-tags lxstamp_gpu`). On older kernels, Landlock and seccomp warn and continue instead of aborting.
- CI fails if `AllThreadsSyscall` is unusable. It used to skip.
- Concurrent status calls no longer race on the packets-per-second sample.
- The io_uring probe smashed an 84-byte stack struct. The kernel writes 120 bytes.
- rngit says NOT_FOUND, not DISALLOWED, when the remote cannot read the repo.
- Deleting a work document fails if `.allowed` is missing, same as Python.
- `gperms` requires a group.
- `SanRef` only rejects a bare `@` and `@{`.
- Mirror and fork clones land in a temp dir, accept only rns/http/https/ssh sources, and reload permissions after.
- Mirror sync points HEAD at the upstream default branch. Fork sync leaves HEAD alone.
- Permission reloads are atomic and apply immediately.
- File responses keep the metadata Python clients use to match a finished transfer.
- The server config has a log level, and failed requests show up on the debug facility.
- rgosh logs a dropped stream chunk instead of going quiet.
- The long rgosh end-to-end test retries once when the watchdog starves under parallel load.
- Eight timed-out requests in a row used to wedge the link. Timed-out receipts are dropped, so the slot frees.
- A request timeout no longer fires in the middle of a response transfer.
- An incoming resource gives up after 16 stalls, same as Python, instead of retrying forever.
- Replacing an in-flight resource releases the old buffers and the old receipt.
- Keepalives are plaintext, matching Python. Encrypted ones were ignored, and the link went stale.
- The second segment of a split response no longer aborts the transfer.

## v1.1.0 - 2026-08-30

Wire compatible with Python RNS 1.5.4.

### Added

- `reticulum-go zen`: a static scanner for path and link mistakes, with optional safe fixes.
- More CGO-free release targets (Linux, BSD, Solaris, illumos, AIX, Android). Linux amd64 ships v1 and v3.
- Local LXStamper proof-of-work on discovery announces.
- Blackhole federation: publish, remote sources, periodic merge.
- Discovery operator LXMF address, and a NomadNet page field.
- Discovery transport implementation and version fields (RNS 1.5.1+).
- `rgostatus` flags from RNS 1.5.0, including queue view and active link count.
- Per-interface counters for protocol, IFAC, and filter violations, plus announce and path-request bytes.
- Inbound priority queues from RNS 1.5.0, with lengths in the config and pressure in the stats.
- `rgostatus -d`/`-D` lists discovered interfaces, persisted by discovery hash.
- `reticulum-go git` and `git-remote-rns`, with Python interop tests.

### Changed

- Path and link timeouts, throttling, and relay follow RNS 1.4.2.
- Path-request wait scales from the slowest interface that is actually up.
- Link setup allows 6 seconds per hop. Duplicate or busy handshakes return a real error.
- The `slim` build tag drops QUIC, WebTransport, I2P, and SDR.
- Default log level is info. Hot paths skip work when that line would be filtered.
- Control API and librns path requests report how long they waited, and wait for `AwaitPath` before opening a link.
- `link_count` is the size of the link table. `active_links` counts only validated rows.
- Default inbound queues match RNS 1.5.1+ (1024/128/128/8).
- Interop and CI track Python RNS 1.5.4.

### Fixed

- `known_destinations` on disk uses raw 16-byte hashes, so Python can load a file Go wrote.
- Shared-instance clients still get path and link relay when transport is off.
- Path and link relay failures return an error instead of vanishing.
- `AwaitPath` timeout names the destination that had no path.
- Transmit packet counters stay in step with byte counters.
- CI bench and fuzz jobs stop when they should.
- Channel retry timing matches Python.
- `dos_protection` defaults to off. The core router no longer forces prevent mode.
- `drop_announce_queues` clears interface queues, not the path cache.

### Tests and docs

- Golden RNS 1.4.2 vectors for packets, announces, channels, resources, and links.
- Configuration, transport, links, compatibility, and development docs updated.

## v1.0.2 - 2026-08-14

Wire compatible with Python RNS 1.4.2.

### Added

- Path-request and link waits follow the slowest radio that is up, with a 5 bit/s floor, instead of a flat 15 seconds. Offline interfaces are skipped. A non-positive bitrate counts as not ready.
- `bitrate` is applied when the interface is created, so that math sees the radio.
- First-hop timeout matches Python airtime, including over RPC. Link establishment timeout is wired through the CLI tools.
- Discovery drops blackholed transport ids and announcer identities when the announce arrives.
- Identity ratchets on SINGLE announce, remember, and encrypt. Public keys are stored the way Python stores them. Pageserver ratchet paths use `{destination_hash}`.
- librns can turn ratchets on. The same helpers are in the C, Odin, Zig, C++, Dart, Rust, Python, Lua, Swift, Java, and Kotlin bindings.
- GROUP destinations with a Token PSK, AES-256, local broadcast, and a one-hop drop. Live-tested against Python.
- `reticulum-go sh` (rgosh): interactive remote shell, PTY sessions, Ctrl-C, `~.` / `~L` / `~?`, announce every 15 minutes by default. `--compat` forces the Python rnsh destination. Windows falls back to a pipe.
- Remote `rgopath` and `rgostatus` (`-R` / `-i`) when `enable_remote_management` is set.
- `dos_protection` shows up in status JSON, the control API, and rgoslow. Config knobs for the caps, bitrate-scaled floors, and priority shedding.
- Shared listeners (UDP, TCP, QUIC, VSOCK, HTTPS, I2P) give each peer its own admit budget, so one sender cannot cool the whole interface.
- Ingress overflow always sheds. Packet handling is a pool of 512 workers, not a goroutine per packet.
- Stream reads are 64 KiB. HDLC framing on the wire is unchanged.
- `node_profile` (`core_router` or `embedded`) fills only the knobs you left unset.
- FreeBSD sandbox re-execs on SIGHUP so a reload works under Capsicum.
- Linux sandbox uses go-landlock. Landlock and seccomp warn and continue on failure. Extra paths can be allowlisted, including a strict profile and a control-API socket.
- systemd hardening example, with an optional User= drop-in.
- Pageserver and `rgocp` fetch resolve symlinks before the path jail.
- deb, rpm, and Arch packages, with tool symlinks, man pages, and a systemd hook.
- Release builds for Windows XP and Server 2003, plus 386 and riscv64 where those OS builds exist.
- Linux ppc64 special baud rates. DragonFly TCP keepalive options.
- `.rsm` inventory generator.
- The vendored protocols module was renamed from reticulum-go-mf to reticulum-go-protocols.

### Changed

- Announce ingest at info log level is about as cheap as critical (around 5 allocations).
- Known destinations sit in RAM as structs. Disk keys stay 16-byte msgpack so Python can read them. Old hex keys still load.
- Link encrypt uses one output buffer. The backbone HDLC assembler goes idle at 64 KiB.
- An incoming handshake counts against `MaxRegisteredLinks` until it is accepted or refused.
- The tunnel table drops expired rows on insert and stops at 256 live tunnels.
- Channel send-window grow and shrink follows Python, including ready-to-send and MDU.

### Fixed

- rgosh killed the remote process after 5 seconds. Interactive shells stay up.
- A path request no longer fans out to an interface that went offline while discovery drained.
- A negative RESOURCE_HMU hashmap segment panicked the process. Indexes that overflow a multiply no longer wrap into earlier slots.
- Tunnel expiry was a bare integer of nanoseconds. It is 8 hours.
- Channel timeouts no longer fire while the lock is dropped. Delivery waits for the link proof, in order, with duplicates dropped.
- `Channel.Send` refuses a full window and any envelope bigger than the outlet MDU.
- `GetCurrentRatchetKey` no longer mints a key on read. On-wire SINGLE ratchets live on the destination.
- A local announce requires IN mode and skips access-point interfaces. `Remember` rejects two identities claiming one destination hash.
- Concurrent link-identify packets no longer race the remote identity.

## v1.0.1 - 2026-07-25

Wire compatible with Python RNS 1.4.1.

### Added

- librns bindings for Rust, Python, Lua, Swift, Java, and Kotlin, each with a pageserver example.
- librns ABI 1.5: sign, verify, public key, RSG, and sending a packet from C.
- Go-only `dos_protection` (off, detect, prevent, auto). Adaptive baseline, persisted learning, per-interface cool-down, and budgets for crypto and handshakes. Default in this release was auto.
- Interface gravity, LRPROOF rebalance, internal announces, boundary search, and request size caps from RNS 1.4.1.
- A live-interface penalty on gravity contests, and a refusal to hop-increase a path that LRPROOF just rebalanced.
- Backbone blocked-IP list in stats, status, and the control API.
- Background cleaning of known destinations.
- Keepalives still go out when the peer is sending and you are only receiving.
- `SetMaxRequestSize` on a destination, and `RequestLimited` on a link.

### Fixed

- Link requests are registered before send, so a fast reply is not dropped.
- Response callbacks still run if you attach them after the request finished.
- UDP receive counters count bytes and packets.
- IFAC on the base interface follows the shared ingress policy.
- Python, Rust, and Lua event polls allocate the app buffer, so payloads are not cut off.
- The link watchdog sends a keepalive even when the remote is transmitting continuously (RNS 1.4.0).
- Backbone client count and blocked IPs show up in the RPC stats.

## v1.0.0 - 2026-07-19

First stable release.

Wire compatible with Python RNS 1.3.9.

### Included

- Crypto, identity, destinations, packets, transport, links, channels, buffers, and resources.
- IFAC on UDP, TCP, Auto, and the other interfaces that carry it.
- Interfaces: UDP, TCP, Auto, I2P, Backbone, Pipe, Local, Serial, Modem73, SDR, WebSocket, QUIC, WebTransport, DNS rendezvous, VSOCK, HTTPS.
- Tools: status, id, probe, path, cp, x, pageserver, slow, speedtest, self-check.
- librns C ABI (API 1.4), control API, and sandbox. Bindings for Odin, Zig, C++, and Dart. Cross-builds for a Windows DLL and a macOS dylib.
- In-memory storage with soft caps. Identity save, resource transfers, and resource events for librns hosts.
- Discovery announces for TCP, Backbone, and I2P. Path and probe health counters.
- Examples for announce, link, resources, file transfer, and echo, including pageserver examples in C, Odin, Zig, and C++.
- Go-only underlays: DNS TXT rendezvous, Linux VSOCK, HTTPS long-poll.
- Channel and stream messages match Python on the wire. Live interop for channel, buffer, rncp, and blackhole link identify.
- Shared-instance Unix socket on Linux, with TCP if the shared-instance type is unset.

### Not in this release

- RNode, KISS, AX.25, and Weave.
- Discovery autoconnect.
- Blackhole publish federation.
- Python rnsh (use rgosh), rnir, and rnpkg.
- Remote rnpath and rnstransport.
