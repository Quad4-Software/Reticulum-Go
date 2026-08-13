# Security

## Purpose

This page summarizes security practices for Reticulum-Go. The repository root [SECURITY.md](../../SECURITY.md) remains the authoritative source for reporting contacts and detailed CI paths. [Cryptography](cryptography.md) covers algorithms and key handling.

## Reporting vulnerabilities

Report security issues privately before public disclosure.

| Channel | Contact |
|---------|---------|
| Reticulum LXMF | f489752fbef161c64d65e385a4e9fc74 |
| Email | `security@quad4.io` |

Include enough detail to reproduce the issue: component, expected behavior, actual behavior, and environment.

## Threat model (practical summary)

Reticulum-Go assumes:

- Attackers can send arbitrary packets on configured interfaces
- Long-term identity keys must remain confidential
- Operators configure IFAC passphrases and network names as shared secrets for interface segments
- Host OS hardening is the operator responsibility outside the daemon sandbox

The stack provides cryptographic authentication and encryption per the Reticulum protocol. It does not replace firewall policy, physical security, or application-level authorization.

## Runtime sandbox

The `reticulum-go` daemon calls `sandbox.Apply` from `pkg/sandbox` after config load, transport start, shared-instance attach, and Control API bind. Privileged initialization and listeners complete first so FreeBSD CapEnter and OpenBSD pledge do not block those sockets.

Default: `enable_sandbox = yes` in `[reticulum]`. Set `enable_sandbox = no` to disable (not recommended for production).

On Linux, enable_seccomp defaults to yes when the sandbox is enabled. Set `enable_seccomp = no` to skip the seccomp filter. Install prefers process-wide `SECCOMP_FILTER_FLAG_TSYNC`, then falls back to per-OS-thread install (`AllThreadsSyscall`), then `prctl(PR_SET_SECCOMP)` on kernels without the seccomp syscall. Any remaining failure soft-fails so older kernels and constrained environments keep running.

| OS | Mechanism | Effect |
|----|-----------|--------|
| Linux | Landlock, seccomp-bpf, PR_SET_NO_NEW_PRIVS, rlimits | Whitelists config and storage paths (including `$XDG_RUNTIME_DIR` for Secret Service), denies high-risk syscalls such as ptrace/mount/module load, limits caps |
| OpenBSD | unveil, pledge | Restricts visible paths and syscalls |
| FreeBSD | cap_enter, rlimits | Capability mode after resource limits |
| Darwin | rlimits | Memory, FD, core dump, stack, process limits |
| Windows | Job object | Limits breakaway, processes, working set |
| Other / WASM | no-op | Logs unsupported, continues |

Landlock requires Linux kernel 5.13 or newer. Older kernels skip Landlock gracefully where possible. Seccomp soft-fails on install error or `ENOSYS`.

The WASM build (`reticulum-wasm`) does not use this sandbox. It relies on the browser or host runtime instead.

Sandboxing is defense in depth. It is not a substitute for a MicroVM or strong host isolation. It does not fix weak passphrases, leaked identity files, or misconfigured interfaces.

## Cryptography

All protocol crypto details: [Cryptography](cryptography.md).

Application code should use `pkg/cryptography` and `pkg/identity`. IFAC configuration is part of interface security, not a substitute for link encryption.

## Supply chain

**Vendored dependencies.** Third-party source is committed in `vendor/`. Ordinary builds use `GOFLAGS=-mod=vendor` and `GOPROXY=off` so compile time does not fetch modules from the network.

**CI security scans.** GitHub Actions runs Gosec, govulncheck, and Trivy on pushes and pull requests to `master`/`dev`. CodeQL covers Go, JavaScript/TypeScript, Python, and Actions workflows. Pull requests also run `dependency-review.yml`. Trivy is installed from a pinned release with SHA256 verification (`scripts/ci/setup-trivy.sh`).

**Reproducibility.** CI includes a reproducibility check (`task reproducibility`).

**Actions pinning.** Third-party actions are pinned to full commit SHAs. Dependabot proposes weekly Action updates (`.github/dependabot.yml`).

**Tree integrity.** Root file `reticulum-go.rsm` is an rnid signed message embedding SHA-256 hashes of tracked files (excluding `vendor/` trees). CI verifies signer e46112d44649266d71fe2193e00a4710 and rechecks bytes at job start and end (`make tree-rsm-verify`).

## Releases

Tagged releases publish from `.github/workflows/publish.yml` on GitHub Actions.

Each release asset has a cosign attestation bundle (`*.cosign.bundle`) signed with the project key. Public key: `cosign.pub` in the repository.

Verify:

```bash
sh scripts/ci/verify-release-attestation.sh PATH/TO/blob PATH/TO/blob.cosign.bundle
```

SHA256 listings in release notes are an informal backup. Prefer cosign verification.

SBOMs (SPDX and CycloneDX) are attached to tagged releases via Trivy (`task sbom`).

## Static analysis in development

| Tool | Purpose |
|------|---------|
| Gosec | Go security linter |
| govulncheck | Go vulnerability database with reachability |
| Trivy | Filesystem and dependency scan |
| revive | Style and lint (`make lint`) |

Run locally:

```bash
make vulncheck
make lint
```

Full check target:

```bash
make check
```

## Logging and secrets

- Log destination supports stderr, file, both, syslog, journald, and combinations such as `syslog+stderr`. Set logfile when using a file path.
- High debug levels may print packet hex. Use loglevel 4 or lower in production unless diagnosing an incident.
- rpc_key protects the control API and shared-instance RPC. Generate with cryptographic random bytes. Do not commit keys to version control.
- `identity_backend = secretservice` keeps identity private blobs in the desktop keyring (Secret Service) instead of plaintext files. Requires an unlocked session collection.
- `identity_backend = keyring` stores the same blobs in the Linux kernel keyring (no D-Bus), suitable for systemd units. See [Identity and destinations](identity-and-destinations.md) for threat coverage.
- Identity private keys are held in locked memory when the OS allows (`pkg/securemem`). This is defense in depth, not a substitute for disk encryption or HSM signing.

## DoS protection (local IDS/IPS)

Go-only. Config key `dos_protection` in `[reticulum]` (default `auto`). Implementation: `pkg/protect`.

This is **node-local** overload control. It sheds work on this process so floods, accept storms, crypto spam, handshake spam, resource pile-ups, and memory pressure do not freeze the daemon. It is not a network-wide IDS and does not stop Sybil join storms across the mesh (for example mass fake peers on an anonymity overlay). Pair it with IFAC, careful public-face exposure, and operator policy.

| Mode | Effect |
|------|--------|
| off | No gates |
| detect | Observe and warn only |
| prevent | Observe, warn, and block or shed |
| auto | Learn baselines quietly, persist them, arm prevent, relearn on interface set change or moderate traffic drift |

Adaptive baselines use EWMA of once-per-second peak pps/bps. Flood samples are ignored while learning so an attack cannot become the quiet baseline. Persistence path: `storage/dos_protect.mpack`.

Trips emit rate-limited stdout warnings and increment `dos_*` health counters. Full key reference and gate table: [Configuration](configuration.md#dos_protection-go-only). Tests: [Development and testing](development-and-testing.md#dos_protection-tests).

Handler pool exhaustion always sheds packets (never sync-dispatches on the ingress thread). Priority shedding prefers established link and proof traffic over announce-class floods when slightly over the adaptive trip line, but that leniency still counts toward the interface's cool-down accounting so sustained abuse of it still escalates instead of running indefinitely.

Memory pressure shedding (heap watermark) enforces immediately in `prevent` and in `auto`, regardless of learning phase. It does not wait for `auto` to arm, since heap exhaustion is a safety valve rather than a flood-learning signal. Explicit `detect` still never blocks, matching its observe-only contract.

Rate, byte, and cool-down accounting run per remote peer as well as per interface. A single sender sharing a listener (a busy TCP/QUIC/VSOCK/I2P accept loop, a UDP socket, or the HTTPS long-poll transport) is capped at half of the interface's effective trip line before its own sub-bucket sheds, so one hostile peer cannot exhaust the whole interface budget and cool down every other peer on it. Peer sub-buckets are bounded and idle ones are pruned so the mitigation cannot itself become a memory-growth vector.

On FreeBSD with sandbox enabled, `SIGHUP` re-execs the daemon so `CapEnter` does not block config reload. Other platforms keep in-process `ReloadInterfaces`.

Linux Landlock or seccomp soft-fail emits `WARNING: sandbox soft-unavailable mechanism=...` on stdout once per mechanism per 30 seconds.

## Local mesh health (observe only)

Reticulum-Go keeps node-local integrity and link-health counters in `pkg/health`. They stay on this node. Nothing is flooded to the mesh or sent to a cloud collector.

Counters increment at existing drop and fail sites (IFAC verify, link HMAC, unpack errors, announce signature rejects, link proof rejects, request timestamp skew, blackhole hits, link stale closes, resource stalls, NIC flaps, duplicate announces, suppressed PATH_RESPONSE answers, duplicate path requests, missing announce cache for path answers, and unknown-iface link relay drops). Accept and reject behavior is unchanged.

Operators see the numbers through:

| Surface | What you get |
|---------|----------------|
| `reticulum-go status` | Per-interface integrity totals and fail rate when non-zero |
| `reticulum-go status -json` | Same fields in JSON (ifac_fail, hmac_fail, integrity_fail_rate, stale_closes, announce_dup, …) |
| `reticulum-go snapshot` (rgosnap) | Paths, active links, and full transport health JSON including path drop counters |
| `reticulum-go slow` | Scored findings such as integrity_burst, auth_pressure, link_degraded, ingress_pressure |
| Control API `GET /v1/status` | Integrity and drop fields on each interface object |
| Shared-instance RPC interface_stats | Same msgpack keys (Go daemon only populates them) |

Scoring prefers fail ratios and bitrate-aware thresholds. High latency alone on a low-bitrate radio is not treated as critical. There is no auto blackhole or auto interface offline in this release. The operator decides whether to adjust IFAC keys, enable ingress control, blackhole an identity, or take an interface down.

See [CLI utilities](utilities.md) for status and slow, [packet-debug](packet-debug.md) for dump and snapshot, and [Control API](control-api.md) for HTTP fields.

## Control API exposure

The control API binds to `127.0.0.1` by default. It is disabled unless `enable_control_api = yes`. Do not expose it to untrusted networks without additional TLS and auth layers (not provided by this package).

## BZ2 bomb limits

Resource and buffer decompression enforce size limits aligned with Python 1.1.9 to resist compression bombs. The `rgosh` stream path also caps decompressed channel chunks (`MaxDecompressed`) and rejects oversize inflate attempts.

## rgosh remote shell

`reticulum-go sh` / rgosh authenticates sessions with Reticulum link identity allowlists. Deny tears the link down and never starts a remote process. Per-session config copies prevent argv pollution across clients. Prefer explicit `-a` allowlists over `-n`. Use `-C` when the listener must run a fixed command only. Destination hashes for app `rnsh` select the Python rnsh wire protocol automatically. `--compat` forces that dest and those channel types.

## Hop field validation (RNS 1.3.8)

Python RNS 1.3.8 rejects packets whose hop byte is `>= PATHFINDER_M` (128) during unpack. Reticulum-Go mirrors that in `pkg/packet.Unpack`. Values 128 through 255 are dropped before transport processing.

Link establishment also records expected_hops on both initiator and responder. Initiator LRPROOF acceptance requires the proof hop count to match (or `expected_hops == PATHFINDER_M` when the path length was unknown at link creation), matching Python Transport pending-link gating.

## Related documents

- [Cryptography](cryptography.md)
- [Configuration](configuration.md) for sandbox, control API, and dos_protection keys
- [CLI utilities](utilities.md) for status and slow health findings
- [Packet debug](packet-debug.md) for dump, snapshot, and Wireshark
- [SECURITY.md](../../SECURITY.md) full policy text
- [Compatibility](compatibility.md) for RNS 1.4.2 interop
- [Package map](package-map.md#pkgprotect) for `pkg/protect`
