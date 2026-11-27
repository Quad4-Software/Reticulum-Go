# Changelog

## v1.6.2 - unreleased

### Added

- `reticulum-go completion bash|zsh|fish` prints shell completions generated from the live flag sets. Linux packages install the bash, zsh, and fish scripts.
- `rgosh -p -json`, `rgocp -p -json`, and `rgox -p -json` print full identity and destination hashes as JSON without angle brackets. `rgoid -p -json` prints the identity and public key, and private key material stays gated behind `-P`.
- The rgosh remote-exit timeout now reports whether the link is still active and whether stdout EOF, stderr EOF, and the exit message arrived.
- librns bindings now cover the ABI 1.6 functions for config reload, path refresh, packet send, destination encryption and proof strategy, link lookup, RSG file signing, and event callbacks where the language exposes them. `task check-bindings-abi` checks the wrappers against `include/rns.h`.

### Changed

- Reticulum tool `-config` flags accept a config file or a directory. rgosh and rgox keep parsing multi-letter tool flags immediately after the first positional argument, while single-letter arguments there belong to the remote command. Their `-a` values accept bare or angle-bracketed full hashes and reject truncated hashes with a clear error.
- The protocols module and the msgpack, bzip2, pbt and tagparser dependencies moved to the self-hosted Forgejo at git.quad4.io. Import paths are now `git.quad4.io/reticulum/Reticulum-Go/protocols`, `git.quad4.io/libraries/msgpack/v5` v5.9.3, `git.quad4.io/libraries/bzip2` v1.0.2, `git.quad4.io/libraries/pbt` v1.0.3 and `git.quad4.io/libraries/tagparser/v2` v2.2.2, replacing the github.com/Quad4-Software paths.

### Fixed
- Announces deferred for stamp generation no longer burn burst-limiter budget per `Announce()` call: the per-destination burst limiter now counts packets actually emitted, so a destination whose first announce waits seconds on proof-of-work generation is not throttled when the stamper emits it. The stamper also keeps its dedup key for the whole job including generation, so re-announces during a slow generation queue at most one follow-up instead of a burst of duplicates. Previously a golxmd startup could silently drop most of its staggered re-announces on networks where stamp costs are nonzero.
- golxmd now applies the `[stamps]` section of the Reticulum config passed with `--rnsconfig` to its transport, matching Python rnsd, so stamp costs and enforcement follow the shared network configuration instead of the compiled-in defaults.
- The shared-instance owner now forwards inbound network traffic to destinations registered by its local clients: Python records such destinations in the path table with zero hops pointing at the client interface, and packets for them were silently dropped instead of forwarded, breaking delivery and link establishment to golxmd, rgosh, and rgox clients behind a Go-hosted shared instance. `TestMatrix_DeliverySharedInstance` now passes end to end, and hermetic regression tests cover forwarding, loop prevention, and announce exclusion.
- Stamp generation no longer races the caller's key-material wipe: `GenerateStamp` and `GenerateStampedKey` now join all worker goroutines before returning, fixing a data race on the material buffer found by `go test -race` in the live link establishment path.
- golxmd peer state is now guarded against data races between peer sync workers and concurrent status queries: mutable peer fields are read under a per-peer lock in the stats compiler and written under the same lock in the sync state machine. Found by `go test -race` on the propagation stats path.
- golxmd gained an adversarial, fuzz, and chaos battery covering hostile control/propagation payloads, hostile Python state imports, corrupted-state restarts, store accounting and eviction invariants, crash recovery, and concurrent hostile clients with bounded goroutine and fd counts. New fuzz targets cover the propagation/control request handlers and the Python peers-file importer.

- golxmd no longer panics in the propagation stats reporter. Peer keys on the `/pn/get/stats` wire response are msgpack binary values like Python sends, local status output uses hex keys, and a panic in a control request handler degrades to a logged error instead of killing the daemon.
- golxmd now registers the transport path request handler, so remote nodes can resolve paths to its control destination on demand instead of waiting for periodic announces. Path requests for the delivery and propagation destinations are now answered too: the LXMF router registers itself as the packet receiver for those hashes, which used to replace the destination object transport needed to answer path requests, so the answers were silently dropped and peering or delivery to golxmd only worked when a periodic announce happened to arrive.
- golxmd exits 201 when the query config dir does not exist and 202 when the identity file is missing, matching Python lxmd query-mode exit codes.
- golxmd matches the Python lxmd config and data layout: config dir resolution order `/etc/lxmd`, `~/.config/lxmd`, `~/.lxmd`, the default reticulum config dir `~/.reticulum`, and router state under `storage/lxmf`. On first start against a Python lxmd config dir it imports the Python messagestore and peer list into `storage/lxmf/golxmd` without modifying the Python files. Unknown config keys now log warnings, and `propagation_stamp_throttle` plus the remaining Python lxmd config keys are parsed.
- golxmd announces its control destination at startup when the control allowed list holds more than one entry, matching Python lxmd, which seeds that list with the node's own identity. Without the announce the control destination is unreachable behind transport nodes, since Reticulum does not recursively forward path requests received on full-mode interfaces. golxmd control queries also wait for a network interface to come online before sending path requests instead of dropping the first request on an interface that is still connecting.
- Interfaces in Reticulum config files are now enabled by default in the config parser, matching Python Reticulum. Previously an interface without an explicit `interface_enabled` key was silently skipped, so tools ignored interfaces from Python-written configs that rely on the default.
- The pageserver initializes the backbone hub on start and honors the log level from its config.
- Release asset verification no longer allows the retired Fulcio and Rekor endpoints.
- LXMF message decoding checks that a `uint` msgpack value fits in `int64` before converting, so a large value from an untrusted peer can no longer wrap negative. The router's `sync_strategy` peer setting must be between 0 and 255 instead of silently wrapping when cast to a byte.
- Dead stores and about 20 unused functions and fields in the protocols module are removed, and unchecked errors across both modules are now handled. `gofmt`, `go vet`, `gosec` and `golangci-lint` report no findings on the root and protocols modules.

## v1.6.1 - 2026-11-21

Tracks Python RNS 1.6.1 plus post-release fixes.

### Added

- The protocols module (LXMF, LXST, MF, RRC, their bindings and daemons) and the `hugo-reticulum` docs site now live in this repository under `protocols/` and `website/`. Both modules replace the root module with a local path. New `task` targets build and test each module, and new CI jobs cover them. `make vendor-sync` now reads the protocols module from `protocols/` instead of the `Reticulum-Go-Deps` sibling tree.
- `rgostatus --memory` prints Go runtime memory stats (heap, stack, sys, goroutines, GC counters, soft limit) and transport table sizes. The data comes from a new `memory_stats` shared-instance RPC call, which Python instances answer as unavailable.
- `SIGUSR1` on Unix writes a heap profile to `<config>/storage/heap-<ts>.pprof` and keeps the newest five. The daemon warns when the heap crosses 85 percent of the soft memory limit, and `RETICULUM_HEAP_DUMP_ON_THRESHOLD=1` writes a profile on each crossing.
- `task release-rngit TAG=<tag>` publishes a tagged GitHub release to the rngit repository at `rns://06a54b505bb67b25ef3f8097e8001edc/public/Reticulum-Go`. Every asset gets a `.rsg` signature, the release carries a `manifest.rsm`, and the notes are taken from the matching changelog section. `ARGS=--dry-run` builds all of it locally without uploading.

### Fixed

- A `discoverable = yes` interface on a non-transport instance no longer announces under the ephemeral transport identity. Upstream commit `606a1691` made that interface non-discoverable and logged a warning pointing at `static_transport_identity = yes`. Go used to enable the static identity on its own, which was the behavior upstream reverted.
- `TRANSPORT_VERS` in tunnel synthesis and discovery announces always carries a version string Python's `version_tuple` can parse. Release builds send the bare tag form (`1.6.0`), in-tree builds send the module pseudo-version, and an unparseable value falls back to `0.0.0-dev`. Before this, a plain build sent `dev` and a release build sent `v1.6.0`, both of which Python treats as a protocol violation. Received metadata and autoconnect qualification now parse versions as strictly as Python does.
- The rgosh end-to-end UDP peer configs set a low destination stamp cost, so stamp generation finishes inside the announce wait on loaded runners.
- Known destinations store the announce payload hash and the raw public key, not a full announce packet copy and a per-destination `Identity`. This matches the Python `known_destinations` layout, where field 1 is `packet_hash`. Retained heap drops by about 700 bytes per known destination, and the sqlite `packet_hash` column and the `known_destinations` RPC export carry the 32 byte hash Python expects.
- The in-memory announce packet cache keeps the slim wire fields needed to rebuild a PATH_RESPONSE announce. That saves about 150 bytes per cached announce.
- The path table, announce packet cache and seen-announce maps are rebuilt once they fall below half of their observed peak, because Go maps keep their bucket allocation after large culls.
- Over-cap path table eviction picks the oldest batch in one pass. The old code rescanned the table per insert, which cost about 2 ms per announce at a full 350k table. It is about 0.6 ms now.
- Path table persistence streams to a temporary file through a buffered msgpack encoder. The old code marshaled the whole table into a second in-memory copy on every save.
- A remote stdin EOF on an rgosh PTY session writes EOT instead of closing the PTY master. Closing it SIGHUPed the child, so a fast command exited by signal, Go reported -1, and a Python rnsh client mirrored that as exit status 255. The master closes after a 250 ms grace, which matches Python's `close_stdin` plus `ensure_terminate` window. Signaled exits are reported as the Python `waitpid & 0xff` code.
- An rgosh listener defers link teardown up to `max(rtt*3, 6s)` after the exit-status packet drains its TX ring, like Python's session prune. The peer can then consume the final stream and exit messages before the link drops.
- `pkg/securemem` keeps secrets off the Go heap. Buffers come from shared mmap'd arenas with PROT_NONE guard pages, best-effort mlock, `MADV_DONTDUMP` and `MADV_WIPEONFORK` on Linux, `MAP_CONCEAL` on OpenBSD, and `VirtualAlloc`, `VirtualLock` and `VirtualProtect` on Windows. Cells carry canaries, `Close` wipes, and a finalizer wipes buffers that were never closed. `NewProtected` returns a dedicated mapping that stays PROT_NONE except inside the scoped `Use` accessor. Small secrets allocate from a slab, so a link key no longer costs an mlock syscall per buffer. Allocation and release went from about 6.1 microseconds to 0.37 microseconds. The daemon turns off core dumps and dumpability at startup (`RETICULUM_DISABLE_HARDEN=1` opts out), and `securemem.Do` wraps `runtime/secret.Do` when built with `goexperiment.runtimesecret`.

### Changed

- Go module dependencies refreshed: x/crypto v0.59.0, x/sys v0.50.0, x/term v0.48.0, x/sync v0.25.0, x/text v0.44.0, x/mod v0.43.0, go-landlock v0.11.2, vsock v1.4.0, yaml v3.0.6, mock v0.6.0. quic-go stays at v0.64.1: v0.65.0 changes the http3 `HTTPStreamer` API and the vendored webtransport-go v0.13.0 has no release that handles it. The Go toolchain stays at 1.27.4, the latest patch release. Dev tool pins are aligned across `mise.toml`, `scripts/ci/versions.env`, `scripts/ci/dev-tools.env` and the workflows at task 3.54.0, revive v1.17.1, staticcheck v0.8.2 and govulncheck v1.9.0, so the portable bootstrap installs what CI runs. GitHub Actions stay pinned by commit SHA.
- With `soft_memory_limit` unset, the daemon applies a soft heap limit of about 65 percent of the cgroup limit or total RAM. Freed transport memory goes back to the OS instead of sitting in the GOGC=100 headroom. `GOMEMLIMIT` and an explicit `soft_memory_limit` take precedence, and a limit set earlier in the process is left alone.
- Removed unused functions, fields, constants and dead assignments found by staticcheck U1000, SA4006, SA4010, SA9003, unparam and ineffassign. A `task deadcode` check and a CI lint step fail on newly introduced unused code.
- The tree manifest `reticulum-go.rsm` is signed by `6947fc2980d273b51a8ff4bc22b290b1`, replacing `e318cbc04468bd574db2b4523dddd710`.

## v1.6.0 - 2026-11-20

Tracks Python RNS 1.6.1.

### Changed

- Go module path moved to `git.quad4.io/reticulum/Reticulum-Go`. Update imports and `go get`/`go install` commands accordingly. The forge serves `go-import` meta tags so the path resolves directly.
- Tracked git hooks (`.githooks/`), the GitHub issue forms, and the pull request template are removed. DCO sign-off is still checked by the `dco-signoff` CI job, and `reticulum-go.rsm` is still maintained manually via `make tree-rsm-sign`.
- The `.crow` and `.gitea` workflow directories and the automated CI flake reporter are removed. Test summaries and the generic retry pass remain, and flaky-test tracking is now manual instead of auto-filed issues.
- Install docs now lead with `git clone https://git.quad4.io/reticulum/Reticulum-Go.git` plus `make build`/`make install`. The curl-pipe examples are gone from the README and getting-started guide.
- `enable_transport` now defaults to `no`, matching the Python RNS default. Only stationary always-on systems should be transport nodes. Fresh installs no longer join the mesh as routers unless configured. Existing config files are unaffected because the value is written explicitly. Set `enable_transport = yes` for backbone/transport deployments.
- Per-destination egress announce rate history is retained for the reference `RATE_TABLE_RETAIN` window (one hour, or the configured target when larger) instead of a flat 24 hours, and expired entries are now culled periodically. Previously the table was only pruned lazily per destination on access and `CleanupExpired` had no caller.
- Announce deduplication no longer drops alternate copies outright. A seen announce is evaluated against the path contest on cleartext fields (hops, receiving interface, random blob) before verification: copies that cannot improve the route still cost only one SHA-256 and a map lookup, while copies that could win (higher interface gravity, unresponsive path, newer emission) pay for signature verification and re-run the full contest, matching the reference implementation which verifies every copy. Verified announce copies that lose the post-verification contest no longer propagate: mesh forwarding, local-client delivery, announce handlers, path-request answers, and the announce packet cache are now gated on the contest result exactly as in Python `Transport`.
- Held announces that answer a destination with a pending local path request are released ahead of unrelated held announces on each ingress-control drain. Priority releases still respect burst and hold/penalty gates but skip the 30-second FIFO pacing interval, so path-request latency during announce floods drops from up to the release interval to the next maintenance tick.

### Fixed

- A deferred stamped announce copied its `tag` reference instead of its bytes, so the job could read a pooled inbound packet buffer after the buffer was recycled, corrupting the path-response tag log and racing the next inbound copy. The queued job now owns a copy of the tag.
- Cancelling a pending announce forward after hearing a rebroadcast returned the pooled packet buffer while a due-job copy could still be mid-forward on other interfaces, and its interface-dedup map was shared with the pending copy. Removed buffers are no longer recycled while in flight and due jobs carry a cloned dedup map.
- Path culling and link-establishment expiry no longer remove this instance's own local destination paths (hops == 0), matching the Python 1.6.1 fix for the 1.6.0 regression.
- `rate.AnnounceRateControl.AllowAnnounceHash` no longer panics on an empty history when grace is zero.
- Removed dead code: the unused `AnnounceManager`/`AnnounceEntry` announce queue (`pkg/transport/announce.go`) and the constants only it referenced (`MaxRetries`, `RetryInterval`, `MaxQueueSize`, `MinPriorityDelta`, `DefaultPropagationRate`), the unused `AnnounceTimeout` constant, and the unused `rate.DefaultAnnounceRate*` constants. The live announce path (`handleAnnouncePacket` plus delayed forwarding) is unchanged.
- The egress announce-rate grace fallback now uses the same default (5) as the transport-default path instead of a divergent constant (3).
- The incoming-resource watchdog now re-requests an overdue hashmap update (HMU) even while parts are still arriving. Previously the stall check only fired after full transfer idleness, so a lost HMU left the receiver silent for the remainder of the window. On slow or deeply queued paths that silence could outlive the peer's keepalive and get the link marked stale mid-transfer. The re-request leaves in-flight part reservations and window state untouched.
- `rnsgit` media conversion now sniffs the input image format before invoking an external encoder, rejects input that is not a supported image, caps conversion input at 32 MiB, and passes an explicit input format specification to ImageMagick-family backends instead of an unconstrained stdin. This ports the upstream RNS 1.6.1 hotfix (wheel respin `rns-1.6.1-0`, published 2026-11-19) that blocks arbitrary file reads through crafted media input.
- Backbone client interfaces now synthesize a tunnel on outbound connects. Previously only the server side of a backbone link offered one, so a Go client attached to a Python or Go backbone never received the relay's announce stream.
- The persistent announce cache now creates its `cache` directory on first use. On a fresh config the database open failed silently and every announce fell back to memory-only caching.
- Per-interface health counters are released when an interface unregisters. Spawned client names embed the peer key, so every disconnect and reconnect cycle previously left a permanent counter slot behind.
- A spawned interface that failed to attach or register no longer keeps its `client_limit` slot on the parent backbone listener.
- `[stamps] max_workers` is now a cap on the computed worker count instead of an absolute count, so oversized values no longer try to spawn a matching number of mining goroutines.
- The `announce_rate_table` (RNSART) loader bounds its length check without integer overflow, so a crafted entry count can no longer wrap the size product on 32-bit platforms.
- When a held-announce queue is full the transport now releases the announce that would propagate furthest and culls the losing entries, matching the Python selection rule. Previously the queue position decided what was released first.

### Performance

- Established-link DATA packets now process on sharded lanes keyed by link id instead of one serial worker. Packets for a given link keep strict FIFO order while different links decrypt and dispatch concurrently, removing the single-threaded link-data bottleneck under multi-link load.
- Inbound `packet.Packet` structs are pooled and recycled after dispatch and on every preprocess drop path, on top of the existing pooled raw-buffer copies. Removes one struct allocation per inbound packet.
- Ratchet public-key derivation is memoized per private scalar (SHA-256 keyed cache holding only public values), and ratchet decrypt attempts reuse the identity salt and context instead of recomputing them per attempt. Each ratcheted inbound message previously paid an X25519 scalar multiplication per stored ratchet just to compute the ratchet id.
- Packet handler pools no longer ramp to the full configured depth by default: worker growth is capped at `max(32, GOMAXPROCS*8)` unless `max_packet_handlers` is set explicitly. Queue depth is unchanged. In a 16-node simulated packet flood this cut live handler goroutines from about 8400 to 1200 and heap growth under load by roughly 45%.
- Repeated "no path to relay link request", "no destination for link request", and "exceeds MaxHops" diagnostics are throttled per destination and interface for 30 seconds. A link-request flood previously emitted a full log record per packet, over 32000 records in an 8-second simulated run, serializing on the log handler.
- Announce processing runs on dedicated lanes sharded by announced destination hash (bounded count scaled to GOMAXPROCS) instead of the shared packet-handler pool. Per-destination announce ordering is preserved while unrelated announces verify in parallel, and an announce flood can no longer occupy every packet handler or convoy on the global known-destination lock. Combined with the per-class inbound queues this keeps floods isolated to the announce class: link DATA and path requests keep their own bounded queues and drain first.
- Inbound job queues are fixed-capacity ring buffers. The previous slice implementation popped from the front and resliced on every dequeue, which reallocated and copied under sustained load.
- Bounded maps (seen announces, announce packet cache, link relay table, reverse proof table) now evict in timestamp-sorted batches down to roughly 90% of capacity instead of scanning the whole map for a single oldest entry on every insert once at the cap.
- Health rate-window counters increment atomically with a mutex taken only on bucket rollover (about 9ns/op, previously a mutex per increment), and `TouchKnownDestination` fast-paths destinations already touched within the current second under a read lock.
- Packet SHA-256 is computed lazily on first `GetHash` instead of eagerly inside `Unpack`, so packets rejected by early transport-id and format filters skip the hash entirely.
- Per-packet-job profiler spans are now opt-in via `enable_packet_profiling` in the config (off by default). Each span cost hundreds of nanoseconds plus allocations per inbound packet under profiling.

### Added

- RNS 1.6.1 parity work. Destination and path-request stamp generation are on by default (`STAMP_DESTINATIONS`, `STAMP_PRS`), so first start and each announce spend CPU mining stamps at the configured costs (default destination cost 25 takes roughly a second on a fast core and noticeably longer on small machines). Stamp costs quantize like Python `quantize_cost`, and `[stamps]` accepts `max_workers` and `worker_niceness` (niceness is parsed and clamped 0-20 but Go has no portable thread priority, so it is stored only). `[filter]` entries now resolve destination types by name hash and accept `<hex>` hash references with Python's truncation and fallback rules, `hop_limiter` works globally and per interface, and announce ingress runs the 1.6.1 early checks: packet size guards, per-interface then global hop limiter, blocked and required-only type gates, and the announce prefilter against the known path (emission timebase, interface gravity, unresponsive state) before any signature work. Losing copies still feed the failover alternate buffer. Filtered traffic is accounted per interface and transport-wide (`frxb`/`ftxb`/`frxs`/`ftxs`/`afrxc`, `lrxb`/`ltxb`/`lrxs`/`ltxs`) and exposed over interface stats RPC along with `tunnel_id`, `remote_transport_id`, and remote implementation/version/stamp metadata (`rem_impl`, `rem_vers`, `rem_is_tn`, `rem_dst_cost`, `rem_pr_cost`). Path table RPC rows carry `interface_hash` and `state`. Tunnel synthesis is bidirectional on non shared-medium interfaces, signs the ext payload carrying implementation name, version, transport flag, and interface stamp costs, validates the same fields on receipt, drops blackholed remote transports, and caps tunnel expiry on load at the 10-minute reconnect window. A `blocked_ips` file in the config dir permanently blocks backbone peers, and `autoconnect_discovered_interfaces` warns above 6 and caps at 12 unless `autoconnect_discovered_interfaces_disable_caps` is set.
- Adaptive receive-side congestion control for incoming Resources (`pkg/link/incoming_resource_cc.go`). Each request round is measured (bytes over elapsed time, request-to-first-part RTT) and the link is classified with round-count hysteresis matching Python `Resource.py`: the window ceiling is 4 parts below 250 B/s, 10 parts by default, and 75 parts after sustained rounds above 6250 B/s, promoting or demoting over consecutive healthy or very-slow rounds instead of starting every transfer at the fast ceiling. The stall grace is derived from the smoothed RTT and measured rate (outstanding bytes over rate, margin factor, one RTT, per-stall backoff) instead of a fixed 250 ms, and interfaces that perform their own loss recovery (TCP, QUIC, backbone, VSOCK, I2P, Unix, HTTPS, WebTransport) double the margin so transport retransmits are not mistaken for stalls. The measured rate and class are carried across transfers on a link so back-to-back files skip the slow-start ramp. Covered by unit tests and in-memory impaired-pipe transfer tests (loss, delay, throttle, mid-transfer regime changes) under `-race`. The wire format is unchanged and Go-to-Python resource interop still passes in both directions.
- RNS 1.6.0 path table sharding. Every outbound packet routed over a path now marks it used (`Path.MarkUsed`, Python `IDX_PT_USED` in `Transport.outbound`, announces excluded), and the 5-second path cull implements the rest of the sharding semantics from Python `Transport`: paths whose attached interface disappeared are removed, never-used paths are culled after the unused-path linger (1h45m on transport nodes, 12h on leaf instances, ×10 on interfaces under 25 kbit/s), and when the never-used pool exceeds its target the oldest entries are culled down to the target, non-slow interfaces first and earliest expiry next. Previously expired paths were only dropped lazily on access, never-used paths lived out the full seven-day `PATHFINDER_E` lifetime, and paths whose interface vanished stayed in the table. Covered by unit tests for marking, linger, slow-interface multiplier, interface removal and pool culling. The persisted path-table format is unchanged (paths loaded from disk start as never-used and follow the linger policy).
- `tests/interop/vega_backbone_live_test.go` (gated by `RUN_VEGA_LIVE=1`): live end-to-end resource transfers over the public Vega backbones at 78.46.92.205, Go-to-Go through the Go backbone and Go-to-Python through the Python backbone in both directions, with a Python peer (`tests/interop/py/backbone_resource_peer.py`) that attaches a standalone Python `BackboneClientInterface` to the same relay. Path acquisition expresses interest with path requests, as required by the RNS 1.6.0 path table sharding and announce filtering (relays no longer broadcast every announce to every client).
- `pkg/transport/prof_load_test.go`: opt-in (`RET_PROF_LOAD=1`) simulated-mesh load harness that sustains mixed announce/data traffic and writes CPU, heap, goroutine, mutex, and block pprof profiles.
- `pkg/transport/link_lane_test.go`: regression coverage proving per-link FIFO ordering while multiple links process concurrently.
- `pkg/transport/announce_lane_test.go`: coverage proving per-destination announce ordering on the new announce lanes and that flood overflow sheds without blocking unrelated destinations.
- `pkg/transport/path_contest_test.go`: coverage for the dedup-hit path contest (worse-route drop, gravity win, unresponsive failover), bounded pending-alternate buffering, backup-path promotion, held-announce priority release, and a three-node mesh simulation of alternate-path failover.
- `pkg/rate` ingress control: `ProcessAnnounceHashDest` records the destination hash on held entries and `ReleaseHeldAnnounceFor` releases the first held announce matching a pending path request out of FIFO order.
- Shared-instance RPC now answers `get=known_destinations` with the Python 1.6.1 response shape (msgpack map keyed by raw destination-hash bytes, eight-field entries), so a Python split-mode client can pull known destinations from a Go shared instance.
- Announce retransmission now mirrors the Python `announce_table` lifecycle: queued announces emit at the random pathfinder window, retry up to `PATHFINDER_R` with the fixed `PATHFINDER_G + PATHFINDER_RW` timeout between emits, count heard rebroadcasts toward `LOCAL_REBROADCASTS_MAX`, and cancel when a relayed copy shows the announce already propagated. Path-request responses reuse the same heard-rebroadcast rules through the pre-verify dedup bypass. Interfaces that delivered an identical wire copy while a forward was pending are deduplicated out of that forward (`dedup_ifs`), and a packet's `blocked_interfaces` field is honoured on forwarding.
- More RNS 1.6.1 parity. `BackboneInterface` enforces `client_limit` (default 500) and frees the slot on disconnect. The path pool uses the new constants (`PATH_POOL_UNUSED_MAX` 1536, `PATH_POOL_UNUSED_MIN` 512, `PATH_POOL_USED_TARGET` 2048) and `REVERSE_TIMEOUT` is 3 minutes. Egress announce-rate state persists across restarts to `announce_rate_table` in the config dir (Python RNSART binary layout, transport nodes only). Held-announce queues drain per interface on the maintenance pass, remote transport nodes can be blackholed directly, protocol violations are tracked and reported, and traffic byte counters use a monotonic clock. `rgostatus` gains `-F` (filtered traffic), `-L` (link traffic share) and `-u` (tunnel ID plus remote transport implementation). `rgopath` gains `-a`, `-u`, `-V`, `-E`, `-M` and `-A`. rngit server and pages configs accept `stamp_value` and `link_cost`.

## v1.5.0 - 2026-11-13

Tracks Python RNS 1.6.0.

### Added

- Network stamps (RNS 1.6.0): stamped identities, destination persistence fields, stamped announces, destination/link-cost APIs, configuration support, stamped path requests and link requests, and enforcement options (`enforce_stamps`, per-type stamp PRs/LRs, announce caching). Python-to-Go interop verified live in both directions.
- SQLite storage (`internal/sqlitestore`, pure-Go `modernc.org/sqlite`, CGO-free, build-tagged off for js/wasip1/tinygo): `known_destinations.db` and `cache/announces.db` with Python-identical schemas, WAL mode, legacy file import, automatic fallback to the legacy layout, and read-only open for shared-instance clients.
- `[filter]` config: `ignored_types`/`allowed_types`/`required_only`/`hop_limiter` destination-type filtering with `wants_destination_type`/`unwants_destination_type`/`unwants_all_destination_types` RPC calls.
- RPC additions: `link_distribution`, path-table `value`/`used`/`lr_cost`, rate-table `hash`, interface/path stats counters (`rxpkts`, `txpkts`, `arxpkts`, `atxpkts`, `prxpkts`, `ptxpkts`, `tlinks`, `alinks`, `plinks`, `tpcount`, `unupcount`, `actpcount`).
- Resource `app_flag` (advertisement flags bit 6) and MTU-aware advertisements: oversized advertisements defer the hashmap (`"m": b""`) and the receiver requests it with the `0xFE` initial-hashmap request.
- Discovery 1.6.0: `DST_COST`/`PR_COST` announce fields, autoconnect cost gating, `AUTOCONNECT_MIN_V` 1.5.7, hop-limit `PATHFINDER_M-1`, `RNS.Stamps` value math (replaces LXStamper integer math).
- Live interop coverage for non-ratchet SINGLE destination DATA in both directions, the wire shape LXMF messages use, driven by a new Python `dest_data_peer`. The interop harness now also finds a repository-root `.venv` when `go test` runs with the package directory as its working directory.

### Fixed

- Stamped link requests no longer have their ephemeral X25519 keys overwritten before packet construction.
- Stamp enforcement no longer bypassed by announces with empty `app_data`.
- Announce-cache DB (`storage/cache/announces.db`) now persists and is flushed/cleaned on Python's cadence. Falls back to in-memory when unavailable.
- AutoInterface discovery works on Windows again: the multicast listener no longer binds a scoped wildcard address (WSAEADDRNOTAVAIL), matching the Python reference which binds the wildcard port and joins the group per interface.
- SQLite storage is limited to platforms supported by modernc.org/sqlite (other targets keep the legacy file layout), and the stores are closed on shutdown so Windows can remove temp dirs cleanly.
- Ingress now drops Header Type 2 non-announce packets whose transport id names a different transport instance, matching Python's packet filter. Go previously let them reach local handling, which could re-originate link requests a Python node would never emit.
- Relayed and local-client path requests now keep the received tag and extension payload and fan out to every eligible interface. A fresh tag per interface let the first hop consume the per-destination throttle, collapsing discovery fanout to a single egress where Python emits on all interfaces. Stamped path request queue entries are keyed per interface so one shared stamp serves the whole fanout.
- Transport-enabled interfaces with no configured `announce_rate_target` now apply Python's defaults of 3600 seconds target, 5 grace, and 0 penalty. Go previously ran with no egress announce rate control in that configuration.
- The interface stats RPC no longer takes a recursive read lock, removing a deadlock when stats are read while the path table is being written.
- The SQLite known-destinations store flushes on close and marks ratchet-only changes dirty, so those updates are not lost on shutdown.

### Security

- Go toolchain raised to 1.27.4 for GO-2026-6859, an HTTP/1 server crash in `net/http` fixed upstream. The control API and interface listeners serve HTTP/1.

### Notes / deviations

- Per-path "used" timestamps are approximated (no dedicated tracking). Path-table sharding/culling and memory-density structures are not implemented.
- Wire-visible differences from Python RNS 1.6.0 are enumerated in `docs/en/deviations.md`.

## v1.4.2 - 2026-11-01

### Fixed

- Endpoint quarantine no longer outlives a network outage. When an interface gains a usable address (wifi reconnects, cable plugged in), every endpoint quarantine, consecutive-failure count, flap history, and escalation level is cleared and dials resume immediately. Detection runs on rtnetlink events and as a 15 second poll while an endpoint waits out a cooldown, so platforms without netlink events recover too. Lifetime counters in interface_stats are kept. `Node.OnNetworkAvailable` also clears all quarantines for hosts that signal network return themselves (Android, librns, control API).
- AutoInterface no longer stays dead after a failed adoption. If a listener or the outbound socket failed to start right after an interface returned (for example while the IPv6 link-local address was still settling), the half-adopted entry was kept and later rescans skipped it. Adoption now rolls back fully so the next rescan retries, and the adopted address list no longer accumulates stale or duplicate entries.

## v1.4.1 - 2026-10-06

Tracks Python RNS 1.5.7.

### Added

- GitHub CI runs the same short self-check on a NetBSD 10.2 VM as FreeBSD and OpenBSD. Local anyvm targets cover FreeBSD, OpenBSD, NetBSD, and HardenedBSD (`task test-freebsd-anyvm` and siblings).
- Release assets now carry GitHub OIDC build-provenance attestations (SLSA Build L3) alongside the cosign bundles. Verify with `gh attestation verify <file> --repo Quad4-Software/Reticulum-Go`.
- Cosign attestation bundles are anchored in the Rekor transparency log, and the SLSA predicate records the source commit in resolvedDependencies.

### Fixed

- Shared-instance RPC now answers medium_path_timeout and lowest_interface_bitrate. Python callers such as rngit previously received msgpack nil, which unpacks to None and crashed numeric comparisons. The timeout matches Python Transport.medium_path_timeout: a two-way MTU airtime on the slowest online outgoing interface plus the per-hop establishment timeout.
- interface_stats entries now carry the full always-present Python key set (mtu, txdrp, txdrb, txstalled, txbuffered, burst_activated/count, pr_burst_activated/count, announce_rate_target/grace/penalty, autoconnect_source, ifac_signature/ifac_size/ifac_netname), so Python tools like rnstatus no longer hit KeyError. IngressControl and BaseInterface now track burst activation timestamps and counts to back them.
- rnsutil GetPathTable always sends max_hops, including nil. Python's rpc_loop reads call["max_hops"] unguarded and silently dropped calls that omitted it, which surfaced as read timeouts.
- New live interop coverage: rpc_surface_probe.py calls every client getter against a Go shared instance and asserts strict numeric replies plus the required per-interface key set. The suite also runs rnstatus against the Go instance and every Go RPC getter plus rgostatus against a real Python rnsd, so RPC surface gaps fail in both directions.

### Changed

- Discovery auto-connect now requires the announcing node to be transport-enabled (`transport` field true in the interface announce), matching RNS 1.5.6 `autoconnect_qualified`. Non-transport announces are skipped even with `autoconnect_unverified_implementations`.
- A configured `discoverable` interface on a non-transport instance now forces `static_transport_identity` on at config-apply time (RNS 1.5.6), so peers can map announces back to the node.
- RNS 1.5.7 resets backbone egress-control counters and the transmit buffer after a stall teardown. Go already gets the equivalent behavior structurally: a torn-down backbone stream is terminal and a reconnect attaches a fresh queue, so there is no stale drain state to clear.

## v1.4.0 - 2026-09-30

Tracks Python RNS 1.5.5.

### Added

- Live interface attach, detach, and reload over the shared-instance RPC `manage` path, matching `rnstatus --attach/--detach/--reload`. `rgostatus` gains `-attach`, `-detach`, and `-reload`. `node.AttachInterface`/`DetachInterface`/`ReloadInterface` expose the same operations and route over RPC when the node is a shared-instance client. Gated by `enable_interface_management` (default yes).
- Discovery auto-connect now requires a recognized implementation and minimum version (`AutoconnectMinVersions`: RNS >= 1.5.2, reticulum-go >= 1.1.0) unless `autoconnect_unverified_implementations` is enabled. Announces persist `impl_name`/`version` and rgostatus discovered listings show them in a Stack field, with `-show-stale` and `-show-unknown` filters matching Python defaults.
- Auto-connect resolves interface-name collisions with sequential names, serializes the exists-check plus spawn section, and stops monitoring interfaces that were manually detached.
- rngit page node: `fmt=mu` download converts markdown blobs to micron with link-scope rewriting, blob pages offer an "as micron" link for .md files, and work-page scope filters show document counts.
- Generated I2P discovery config entries append `.b32.i2p` to peers.
- Interfaces learn `Detach` for TCPClient, AutoInterface, SerialInterface, and spawned-child enumeration on BackboneInterface, AwareInterface, and RNodeMultiInterface for management detach.
- Per-interface announce reject-reason counters (announce_malformed, announce_dest_type, announce_blackholed, announce_key_mismatch, announce_max_hops, announce_held, announce_suppressed) surface through interface_stats, rgostatus human output (Announce line), and JSON.
- Endpoint health tracking for outbound client interfaces (TCP, backbone, QUIC, VSOCK, WebTransport, UDP): sustained dial failures or fast flap bursts quarantine an endpoint into an escalating cooldown (5m base, 1h cap) instead of redialing forever. Exposed as endpoint_dial_failures, endpoint_flaps, endpoint_quarantined, endpoint_quarantine_s in interface_stats.
- `rgostatus -T` renders a path topology view grouped by receiving interface (paths, hop range, next-hop hash, age, expiry). Go-only.
- Announce ingest hot path: signature scratch buffer and announce-forward copies are pooled, and the delayed-forward queue carries struct jobs instead of closures (171 B/op, 1 alloc/op steady-state, down from 437 B/op, 5 allocs/op).
- `Destination.Announce` now warns once per minute when a local app exceeds the 8-announces-per-10s burst limit (previously returned `ErrDestAnnounceThrottled` silently). The warn names the destination and count so operators can find the misbehaving app.
- Misuse warnings normalized across link and path paths: `RequestPath` throttling and `TryBeginOutboundEstablish` busy now warn once per minute per destination instead of logging at verbose level, and `Link` warn-once gating covers Establish-while-busy, Establish-after-settled, request-queue-full, duplicate in-flight request, request timeout, and abandoned mid-transfer response. Retry loops get one warn per minute, not a log flood.

### Fixed

- Serial and RNode initial bring-up failures no longer fail Start. The interface registers and retries in the background like Python 1.5.5.
- Spawned Backbone clients inherit announce_cap along with the other parent properties.
- Literal `None` IFAC netname/passphrase values are warned about and ignored, and previously persisted `"None"` strings are dropped at sanitize time.
- Discoverable interfaces that enable IFAC publishing without configured IFAC credentials now warn and publish no IFAC fields instead of emitting empty fields.

## v1.3.2 - 2026-09-25

### Added

- `AwareInterface`: WiFi Aware (NAN) transport for Android. A host-supplied driver owns the NAN session while each data path spawns an HDLC-framed peer interface, wire-identical to TCPClientInterface.
- `RawChannelReader.WaitReadable` and `ReadContext`: cancellable, event-driven reads that never return (0, nil) on an open stream. Plain `Read` keeps its Python-compatible non-blocking semantics.
- Accepted-connection backstop: even with protection disabled, per-interface accepted conns are counted under a generous ceiling.
- CI now rejects commits with placeholder author identities.

### Fixed

- Held announces are forwarded when released instead of dying on the dedup claim.
- Forged or malformed LRPROOF packets no longer kill pending links.
- Receipt proofs are only consumed when they arrive on the expected interface.
- Header Type 2 link relay uses the parsed destination hash instead of the transport ID, restoring multi-hop link traffic through transport nodes.
- Link packets that must be encrypted are dropped when no session keys exist.
- Link teardown works from every state, wakes all waiters and watchdogs, and clears link-installed path entries.
- Interface restart recreates done channels, so stopped interfaces come back instead of staying dead.
- WebTransport server sessions go through admission. VSOCK and WebTransport clients cannot stack duplicate read loops. Vsock dial is bounded.
- HTTPS DoS bucketing keys on the transport host rather than the self-chosen peer header. The server warns when run without peer_key or IFAC.
- max_reconnect_tries = 0 means never reconnect. It is no longer conflated with an unset value.
- Relay tables, discovery state, peer maps, HDLC assembly, and local path-request state are bounded.
- Atomic state writes fsync file and directory. State-file loads are capped. Ratchet init refuses to clobber unreadable material.
- Service loops, the memory monitor, and shared-instance RPC observe stop signals and deadlines.

## v1.3.1 - 2026-09-25

### Changed

- The contributor license grant is replaced by plain DCO sign-off. The `Signed-off-by:` trailer now certifies only the Developer Certificate of Origin.
- The librns event queue now protects payload-bearing events under overflow: non-priority events such as announces are dropped first, and capacity grew from 256 to 4096.
- Mutation testing is now a per-package CI matrix, cutting the job's wall time from roughly 50 minutes to the slowest single package.
- The amd64 test run now builds with `-vet=all` and uploads a `coverage.out` artifact.

### Added

- librns `rns_node_reload_config` hot-reloads interface blocks from the create-time config path without restarting transport. The C ABI is now 1.6, and the Java binding gains `Node.reloadConfig` plus `packetSend` and `destinationEncrypt` helpers.
- `buffer.WriterOptions` with a `CompressionPolicy`: `CompressionAuto` keeps the Python-compatible compression probes, and `CompressionDisabled` always emits the standard uncompressed stream message. New constructors: `NewRawChannelWriterWithOptions`, `CreateWriterWithOptions`, `CreateBidirectionalBufferWithOptions`. `RawChannelWriter.WriteContext` adds caller-controlled cancellation of the TX-window wait. Wire format is unchanged and Python receivers accept both forms.

- `testsummary` honours `TESTSUMMARY_RERUN_FAILS=N`: failed tests get up to N fresh `-count=1` retries, and tests that pass on retry are reported instead of failing the run.
- A nightly `stress` workflow repeats the timing-sensitive tests so timing problems surface off the merge path.
- All pinned CI toolchain installers (`setup-go`, `setup-node`, `setup-task`, `setup-zig`, `setup-odin`, `setup-kotlin`, `setup-tinygo`, `setup-swift`) now verify the resolved binary reports the pinned version and fail loudly on shadowing.
- A `merge-to-master` dispatch workflow merges dev into master only when the latest CI run for the dev head is green.
- Optional RNE1 passphrase-encrypted identity files. Argon2id plus XChaCha20-Poly1305 wrap the standard 64-byte blob, unlocked by prompt, RETICULUM_IDENTITY_PASSPHRASE, a passphrase fd, or an OS wrap store (Linux kernel keyring and Secret Service, macOS Keychain, Windows DPAPI). rgoid gains -to-passphrase, -to-wrapped, -rekey, and -to-file decryption. Local storage format only, no wire change, and decryption always restores the standard file.

### Security

- A failed unlock of an encrypted transport identity no longer falls through to writing a fresh plaintext identity over the file. Startup fails with the real error instead.
- A responder link no longer processes link-request proofs: a reflected copy of its own signed proof could overwrite the peer ephemeral key and fire the established callback on a bogus self-session.
- Backbone interfaces now apply the same connection admission control as every other listener, bound a stream's outbound queue at 8 MiB, and deregister a stream's fd before closing the socket, closing unauthenticated slow-consumer memory exhaustion and an fd-reuse wedge.
- The librns C event callback now copies app_data through C-allocated scratch. Passing a struct containing a Go pointer to C previously tripped cgocheck and aborted the embedding process on any event carrying app_data.
- Interface teardown no longer releases a socket before its poller deregistration, so a reused fd can never be unregistered under a new connection.
- A readable but corrupt transport identity file now fails startup instead of silently replacing the node's identity.
- Ratchet key persistence writes with mode 0600 instead of the umask-derived default, and ingress announce-arrival history is bounded against sustained floods.
- Denied rgosh sessions and finished listener sessions now tear down their links, closing a parked-link exhaustion path against the registered-link ceiling.
- rgosh/rnx/rgocp allowlist loading no longer resolves home-relative paths against the daemon working directory when HOME is unset.
- RNE1 headers now bound accepted Argon2 parameters to 16 rounds, 512 MiB and 16 threads.

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
- Module path is `git.quad4.io/reticulum/Reticulum-Go`. First-party deps are real module versions, and `go.mod` has no `replace` lines (#13).
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
