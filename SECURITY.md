# Security Policy

This document explains how we handle security for Reticulum-Go. It covers how you can report vulnerabilities, how we secure our build process, and how the runtime sandbox protects the running daemon.

For details on cryptographic primitives, key formats, and protocol behavior, please refer to the dedicated guide in [docs/en/cryptography.md](docs/en/cryptography.md). You can also find the full English documentation in [docs/en/](docs/en/README.md).

## Reporting a Vulnerability

If you find a security issue, please contact us privately first. This gives us a chance to investigate and fix the problem before it becomes public.

### How to reach us

*   **LXMF Address:** `f489752fbef161c64d65e385a4e9fc74`
*   **Email:** `security@quad4.io`

When reporting, please include enough detail to help us reproduce or understand the issue. Useful information includes the affected component, what you expected to happen, and what actually happened. We take all reports seriously and will work with you on a reasonable timeline to resolve the issue and coordinate public disclosure.

## Security Practices Overview

Here is a quick look at how we secure the project in practice.

*   **Builds and Automation:** Our continuous integration system runs automated tests and security checks on every change. We use clear, easy to audit shell scripts instead of complex third-party actions. This makes it straightforward to review exactly what runs during a pull request.
*   **Scanning and Analysis:** We run static analysis and vulnerability scanning on our codebase and all dependencies.
*   **Runtime Sandbox:** The `reticulum-go` daemon applies operating system restrictions after it starts up. This limits access to the filesystem and system resources if the process is ever compromised.
*   **Signed Releases:** We attach official **cosign** signatures to all release files. These are signed with a project key, and you can find the public key in `cosign.pub`.

The sections below describe these practices in detail.

## Supply Chain and CI

All of our automation runs on GitHub Actions, with configuration files stored in `.github/workflows/`. 

To run our builds and security scans, we use helper scripts located in `scripts/ci/*.sh`. These scripts install Go, Task, Node, and our security scanners. This approach keeps the setup logic in ordinary shell scripts that you can review and diff.

### Version Pinning

We take supply chain security seriously and use pinning to protect our builds:

*   **Third-party GitHub Actions:** We pin third-party actions to specific commit hashes instead of version tags. This prevents unexpected updates from modifying our build environments. You can see these hashes in the comments at the top of our workflow files.
*   **Build and Security Tools:** We pin the versions of all compilers and scanners inside our workflow environment blocks.

### Source tree integrity (`.rsm`)

The repository root includes a signed rnid message file, `reticulum-go.rsm`. It embeds a SHA-256 inventory of git-tracked files except itself and paths under any `vendor/` tree. CI verifies the signature against the required signer identity `e46112d44649266d71fe2193e00a4710`, then re-hashes file bytes. Jobs also recheck the inventory at the end so a compromised runner cannot silently add or modify tracked files.

Verify locally:

```bash
make tree-rsm-verify
```

Maintainers regenerate the signature after intentional tree changes (requires a private identity file that hashes to the signer above, never commit `*.rid`):

```bash
export RNS_ID_PATH="$HOME/.local/share/reticulum-go/reticulum-go-release.rid"
make tree-rsm-sign
```

Enable the tracked pre-commit hook so commits that change inventory paths resign `reticulum-go.rsm` automatically when that identity is available:

```bash
make hooks-install
```

Skip one commit with `SKIP_TREE_RSM_HOOK=1`.

### Release Provenance and Verification

When we publish a release, we build the binaries, WebAssembly targets, and pageserver examples. We also generate software bills of materials (SBOMs) using Trivy. 

For each release asset, we generate a signed provenance bundle using **cosign**. We do not use separate checksum files.

#### Verifying Release Files

You can verify any release file using our public key and the provided verification script:

```bash
sh scripts/ci/verify-release-attestation.sh path/to/binary path/to/binary.cosign.bundle
```

Alternatively, you can run the standard `cosign` command directly with the same public key and bundle.

### Static Analysis (SAST)

We use several security scanners to check our codebase on every commit:

*   **Gosec:** Scans the Go code for security issues and unsafe coding patterns.
*   **Govulncheck:** Checks our dependencies against the official Go vulnerability database to find reachable vulnerabilities.
*   **Trivy:** Scans our filesystem and dependencies for known vulnerabilities.

#### Our Approach to Scanner Safety

Third-party package managers can be a security risk. In early 2026, some distribution channels for Trivy were compromised when attackers updated workflow tags to distribute malicious binaries. 

To protect our build pipeline, we do not download Trivy using moving tags or unverified URLs. Instead, our installation script downloads a specific release package directly from the Aqua Security release page. We check the package against a hardcoded SHA256 hash before installing it. We update this hash manually when we want to upgrade the scanner.

## Runtime Sandbox

To protect your system, the `reticulum-go` daemon can restrict its own permissions after starting up. It does this by calling `sandbox.Apply` after it has loaded its configuration and set up its network interfaces. 

This sandbox is **enabled by default** using the `enable_sandbox = yes` setting in the Reticulum configuration. If you need to disable it, you can set `enable_sandbox = no`. On Linux, `enable_seccomp = yes` is the default when the sandbox is on. Set `enable_seccomp = no` to skip the seccomp filter. Seccomp install failures soft-fail so the daemon continues.

The level of protection depends on your operating system:

| Operating System | Mechanism | Description |
| :--- | :--- | :--- |
| Linux | Landlock, seccomp-bpf, `PR_SET_NO_NEW_PRIVS`, resource limits | Restricts access to `~/.reticulum-go`, `/tmp`, `$XDG_RUNTIME_DIR`, system configuration folders, and DNS paths. Denies high-risk syscalls. Drops capabilities and runs in a private namespace when started as root. |
| OpenBSD | `unveil`, `pledge` | Restricts visible filesystem paths and limits system calls to only what is needed by a network daemon. |
| FreeBSD | `cap_enter`, resource limits | Enters capability mode and applies conservative resource limits. |
| macOS | Resource limits | Sets limits on memory, file descriptors, stack size, and the number of processes. |
| Windows | Job objects | Limits process creation and memory usage, and disables mini-dumps. |
| Other platforms / WASM | No-op | Logs a warning that sandboxing is not supported on the host platform. |

Landlock requires Linux kernel version 5.13 or newer. On older kernels, Landlock will fail gracefully while other Linux restrictions still apply where possible.

Please note that sandboxing is designed as defense in depth. It is not a replacement for strong cryptography, proper interface configuration, or secure hosting practices. It also does not apply to WebAssembly builds, which rely on the browser or runtime environment sandbox.

## Local DoS protection

Reticulum-Go includes Go-only local overload gates (`dos_protection` in `[reticulum]`, default `auto`). Modes are off, detect, prevent, and auto. They shed floods, accept storms, crypto and handshake spam, resource pile-ups, and memory pressure on **this node** only. They do not ban peers mesh-wide.

Details: [docs/en/security.md](docs/en/security.md#dos-protection-local-idsips) and [docs/en/configuration.md](docs/en/configuration.md#dos_protection-go-only).

## Cryptography

For complete details on our cryptographic algorithms, key formats, identity encryption, and packet validation, please see the guide in [docs/en/cryptography.md](docs/en/cryptography.md). This file is the primary reference for the cryptography used in this project.
