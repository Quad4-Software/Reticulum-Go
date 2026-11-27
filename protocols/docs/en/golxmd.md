# golxmd

golxmd is a drop-in replacement for the Python `lxmd` propagation node daemon. It speaks the same wire protocol, reads the same config format, and imports existing Python lxmd state.

## Config and data layout

The config dir resolution matches Python lxmd: `/etc/lxmd` when it holds a config, then `~/.config/lxmd` when it holds a config, otherwise `~/.lxmd`. The identity lives at `<config>/identity`, so pointing golxmd at an existing Python lxmd config dir reuses the same node identity and the same propagation address.

Router state lives under `<config>/storage/lxmf`, the same layout Python lxmd uses. Delivery messages wait in `<config>/storage/messages`.

## Migrating from Python lxmd

1. Stop the Python lxmd daemon.
2. Start golxmd with `--config` pointing at the Python config dir, or move nothing if it sits in one of the default locations.

On first start golxmd copies the Python messagestore (`storage/lxmf/messagestore`) and the Python peer list (`storage/lxmf/peers`) into `storage/lxmf/golxmd/`. The copy is idempotent and never modifies or deletes the Python files. Invalid entries are skipped with a warning in the logfile, and the configured storage limit is honored during the copy. After the import golxmd serves the imported messages to clients and peers as usual.

To roll back, stop golxmd and start Python lxmd again. The Python state files are untouched.

## Verified interoperability

The live interop suite in `pkg/lxmf` (gated behind `RUN_LIVE_INTEROP=1`) runs golxmd against the real Python lxmd and LXMF client libraries and covers: Python clients fetching from and propagating through golxmd, Python lxmd querying golxmd status and peers and the reverse, delivery to the golxmd local destination with the `on_inbound` hook, propagation peering and message sync in both directions, importing a Python messagestore and peer list, golxmd attaching to a shared Reticulum instance alongside Python lxmd, and a 90 second soak with active status queries. The Python example config parses in golxmd without warnings, and the golxmd example config starts real Python lxmd.

## Known differences

- golxmd keeps its working state in the `golxmd` subdirectory instead of rewriting the Python files in place, so the two daemons never fight over the same files.
- Unknown config keys and sections are logged as warnings instead of being silently ignored.
- State written by golxmd 1.2.0 or older directly under `storage/messagestore` (without the `lxmf` directory) is not imported. That release crash-looped in production, so in practice there is little state of that kind.
- Python lxmd only builds its outbound peer destination when the peer identity is recallable at peer construction time, and peers are constructed at startup. If peering key generation recalls the identity first, every sync fails with "identity could not be recalled". Production Python nodes rely on the persistent identity cache across restarts. In practice: if a fresh Python lxmd never initiates sync to golxmd, restart it once golxmd's identity is in its cache. golxmd is not affected in the other direction.


## Hardening notes

Verified by the adversarial, fuzz, and chaos battery in `pkg/lxmf` (`adversarial_audit_test.go`, `chaos_audit_test.go`, `fuzz_audit_test.go`):

- All control and propagation request handlers (offer, message get, peer sync, unpeer, stats) are total on arbitrary msgpack payloads: no panic, no hang, no unbounded memory, tested with hostile inputs including oversized counts, deep nesting, wrong types, truncated data, and NaN floats. The underlying msgpack decoder rejects declared lengths exceeding the remaining input and caps nesting depth, so small payloads cannot amplify into large allocations.
- Handlers reject unauthorized and identity-less links before any state mutation.
- Propagation transfer size limits are enforced on the resource advertisement before any transfer data is buffered, matching Python lxmd.
- Compressed resource assembly is bounded with a limit reader, so bzip2 bombs cannot expand beyond the advertised and protocol maximum sizes.
- Restarting on corrupted state (truncated peers file, zero-byte or garbage messagestore entries, partial writes) starts cleanly, skipping bad entries.
- The store keeps byte accounting consistent under random insert/remove/reload sequences, never exceeds the configured storage limit after eviction settles, and crash-recovery never loses acknowledged entries or resurrects removed ones.
- The `on_inbound` hook runs synchronously in the delivery path, matching Python lxmd's `subprocess.call` behaviour. A hook script that hangs will stall inbound delivery in both implementations; keep hooks fast. The hook receives the message path as a separate argument (no shell string interpolation), which is stricter than Python's `shlex.split` of a concatenated command line.
- Peer state is guarded against concurrent access between sync workers and status queries.
- Fault-injection chaos testing (`internal/faultproxy`, `interop_chaos_proxy_test.go`) runs golxmd behind a seeded HDLC-frame-aware TCP proxy. Verified: propagation upload converges through the hub under 2% frame drop/duplication/reordering with 5-10ms latency, under repeated connection flaps, and the store holds no corrupt entries afterwards. Seed replay via `LXMF_CHAOS_SEED`. Frame drops at higher compounded rates delay convergence beyond the test budget, same as any lossy link with timer-driven retransmission.
- Kill -9 crash recovery is exercised live over multiple rounds at randomized points including mid-transfer: restart on the same storage keeps the propagation identity, loses no acknowledged store entries, and accepts no corrupt entries.
- Differential testing against the real Python LXMF (`diff_fuzz_test.go`) feeds identical valid, bit-flipped, truncated, extended, and arbitrary wire bytes to both unpackers. Python's `LXMessage.unpack_from_bytes` parses structure only and never verifies the Ed25519 signature; golxmd verifies inline at unpack and is therefore strictly stronger. That is the only accept/reject divergence class and it is intentional: messages with invalid signatures are rejectable garbage either way.
- golxmd's stamp generation API tolerates a nil context (treated as background) instead of panicking.

## Status queries

`golxmd --status [--peers] --remote <hash>` queries a remote propagation node, Python or Go. `lxmd --status --remote <hash>` from the Python tools can query golxmd the same way. The query identity must be listed in the node's `control_allowed` config keys.

Control queries depend on the control destination being reachable. Like Python lxmd, golxmd seeds the allowed list with its own identity and announces the control destination at startup when the list holds more than one entry. This matters behind transport nodes: Reticulum transport nodes do not recursively forward path requests received on full-mode interfaces, so a control destination that is never announced is unreachable through such a hub for Python and Go nodes alike. The shared-instance production topology is not affected because local client path requests are forwarded to all other interfaces.
