# Security

## Purpose

This page summarizes security practices for Reticulum-Go. The repository root [SECURITY.md](../../SECURITY.md) remains the authoritative source for reporting contacts and detailed CI paths. [Cryptography](cryptography.md) covers algorithms and key handling.

## Reporting vulnerabilities

Report security issues privately before public disclosure.

| Channel | Contact |
|---------|---------|
| Reticulum LXMF | `f489752fbef161c64d65e385a4e9fc74` |
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

The `reticulum-go` daemon calls `sandbox.Apply` from `pkg/sandbox` after config load, interface setup, and transport start. Privileged initialization completes first.

Default: `enable_sandbox = yes` in `[reticulum]`. Set `enable_sandbox = no` to disable (not recommended for production).

| OS | Mechanism | Effect |
|----|-----------|--------|
| Linux | Landlock, PR_SET_NO_NEW_PRIVS, rlimits | Whitelists config and storage paths, limits caps |
| OpenBSD | unveil, pledge | Restricts visible paths and syscalls |
| FreeBSD | cap_enter, rlimits | Capability mode after resource limits |
| Darwin | rlimits | Memory, FD, core dump, stack, process limits |
| Windows | Job object | Limits breakaway, processes, working set |
| Other / WASM | no-op | Logs unsupported, continues |

Landlock requires Linux kernel 5.13 or newer. Older kernels skip Landlock gracefully where possible.

The WASM build (`reticulum-wasm`) does not use this sandbox. It relies on the browser or host runtime instead.

Sandboxing is defense in depth. It does not fix weak passphrases, leaked identity files, or misconfigured interfaces.

## Cryptography

All protocol crypto details: [Cryptography](cryptography.md).

Application code should use `pkg/cryptography` and `pkg/identity`. IFAC configuration is part of interface security, not a substitute for link encryption.

## Supply chain

**Vendored dependencies.** Third-party source is committed in `vendor/`. Ordinary builds use `GOFLAGS=-mod=vendor` and `GOPROXY=off` so compile time does not fetch modules from the network.

**CI security scans.** GitHub Actions workflows run Gosec, govulncheck, and Trivy. Trivy is installed from a pinned release with SHA256 verification (`scripts/ci/setup-trivy.sh`).

**Reproducibility.** CI includes a reproducibility check (`task reproducibility`).

**Actions pinning.** GitHub-owned actions are pinned to full commit SHAs where this repository pins them.

**Tree integrity.** Root file `reticulum-go.rsm` is an rnid signed message embedding SHA-256 hashes of tracked files (excluding `vendor/` trees). CI verifies signer `e46112d44649266d71fe2193e00a4710` and rechecks bytes at job start and end (`make tree-rsm-verify`).

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

- Log destination supports `stderr`, `file`, `both`, `syslog`, `journald`, and combinations such as `syslog+stderr`. Set `logfile` when using a file path.
- High debug levels may print packet hex. Use loglevel 4 or lower in production unless diagnosing an incident.
- `rpc_key` protects the control API and shared-instance RPC. Generate with cryptographic random bytes. Do not commit keys to version control.
- `identity_backend = secretservice` keeps identity private blobs in the desktop keyring (Secret Service) instead of plaintext files. Requires an unlocked session collection. See [Identity and destinations](identity-and-destinations.md) for threat coverage.
- Identity private keys are held in locked memory when the OS allows (`pkg/securemem`). This is defense in depth, not a substitute for disk encryption or HSM signing.

## Control API exposure

The control API binds to `127.0.0.1` by default. It is disabled unless `enable_control_api = yes`. Do not expose it to untrusted networks without additional TLS and auth layers (not provided by this package).

## BZ2 bomb limits

Resource and buffer decompression enforce size limits aligned with Python 1.1.9 to resist compression bombs.

## Hop field validation (RNS 1.3.8)

Python RNS 1.3.8 rejects packets whose hop byte is `>= PATHFINDER_M` (128) during unpack. Reticulum-Go mirrors that in `pkg/packet.Unpack`. Values 128 through 255 are dropped before transport processing.

Link establishment also records `expected_hops` on both initiator and responder. Initiator LRPROOF acceptance requires the proof hop count to match (or `expected_hops == PATHFINDER_M` when the path length was unknown at link creation), matching Python `Transport` pending-link gating.

## Related documents

- [Cryptography](cryptography.md)
- [Configuration](configuration.md) for sandbox and control API keys
- [SECURITY.md](../../SECURITY.md) full policy text
- [Compatibility](compatibility.md) for RNS 1.3.8 parity