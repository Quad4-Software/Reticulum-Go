# Security Policy

## Supply Chain Security

- CI is mostly shell-driven because that improves auditability (install and test steps are plain shell you can review and diff in PRs), control (no opaque marketplace-action behavior), updates (bump a pinned version in workflow `env` or adjust a script), and security (explicit tool installs and a smaller dependency surface than long action chains).
- In practice, workflows under `.gitea/workflows/` call `scripts/ci/*.sh` installers (Go, Task, Node, cosign, gosec, govulncheck, Trivy, revive, Python venv, TinyGo, etc.) with pinned versions set in workflow `env`, then run `task` or `go` commands. Repository checkout uses inline `git` shallow-fetch steps (Gitea/GitHub-compatible env), not a third-party checkout action; `scripts/ci/checkout.sh` is available for local or `act` runs. A few steps still use forked actions from https://git.quad4.io/actions (for example artifact upload/download and the Gitea release step in `publish.yml`); those are pinned to a full commit hash, not a moving tag.
- BOM generation using CycloneDX (see `task sbom` and `.gitea/workflows/sbom.yml`)
- Release provenance (SHA256 sidecars, optional SLSA v1 with cosign) is described under [Release provenance](#release-provenance) below.
- Reproducibility is checked in CI (`task reproducibility`, `.gitea/workflows/reproducibility.yml`).

### Release provenance

Tagged releases are built and published from `.gitea/workflows/publish.yml`. CI always produces SHA256 checksum sidecars for release binaries. When the repository secret `COSIGN_PRIVATE_KEY` is set (PEM from `cosign generate-key-pair`, with `COSIGN_PASSWORD` if the key is encrypted), the same workflow also signs artifacts with cosign and attaches SLSA v1 provenance as bundle files (`*.cosign.bundle`) next to each binary.

The cosign public key is committed at the repository root as `cosign.pub`. Verification uses the private-infrastructure path (no Rekor transparency log), which matches self-hosted runners that do not publish to the public log.

To verify a downloaded binary and its bundle with the `cosign` CLI:

`sh scripts/ci/verify-release-attestation.sh path/to/reticulum-go-linux-amd64 path/to/reticulum-go-linux-amd64.cosign.bundle`

Set `COSIGN_PUBLIC_KEY` if your copy of the public key is not named `cosign.pub` in the current directory.

### Static analysis (SAST)

CI runs Gosec (Go security linter), govulncheck (Go vulnerability database, reachable-code analysis for `go.mod` dependencies), and Trivy (filesystem and dependency vulnerability scanning) in `.gitea/workflows/scan.yml`. Gosec is installed with a pinned module version via `scripts/ci/setup-gosec.sh`. Govulncheck is installed with a pinned module version via `scripts/ci/setup-govulncheck.sh`. Trivy is not pulled from upstream GitHub Actions tags or release URLs in the workflow: the job downloads a pinned `.deb` from a repository we control (`TRIVY_DEB_URL` in that workflow), verifies it with `TRIVY_DEB_SHA256`, and installs it through `scripts/ci/setup-trivy.sh`. We bump that URL and hash manually when upgrading Trivy.

That model exists because third-party distribution channels are an attractive supply-chain target. For example, in March 2026 attackers compromised the Trivy ecosystem by repointing most GitHub Action tags in `aquasecurity/trivy-action` and shipping trojanized Trivy binaries through official-looking release and registry paths, so workflows that resolved moving tags or unverified binaries could run malicious code in CI or on developer machines. Pinning a binary we host at an immutable commit URL with a recorded SHA256 avoids depending on those tag or release surfaces for our scans.

## Cryptography Dependencies

- go/crypto
  - ed25519
  - sha256
  - rand
  - aes
  - cipher
  - hmac

- golang.org/x/crypto `v0.50.0`
  - hkdf
  - curve25519

## Reporting a Vulnerability

Please contact me over Reticulum LXMF: `7cc8d66b4f6a0e0e49d34af7f6077b5a` or email `security@quad4.io`.