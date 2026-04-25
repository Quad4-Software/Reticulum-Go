# Security policy

This document explains how we think about security for Reticulum-Go, how to report problems, and where to look if you need implementation-level detail (for example for a security review or internal risk assessment).

## Reporting a vulnerability

If you believe you have found a security issue, please tell us privately first so we can investigate before wider disclosure.

**How to reach us**

- Reticulum LXMF: `7cc8d66b4f6a0e0e49d34af7f6077b5a`
- Email: `security@quad4.io`

Include enough detail to reproduce or understand the issue (what component, what you expected, what happened). We will treat reports seriously and work with you on a sensible timeline for fixes and public communication.

## What we do in practice (overview)

**Builds and automation.** Continuous integration runs tests and security-oriented checks on every change. We prefer clear, reviewable shell scripts over long chains of opaque third-party actions so that what runs in CI is easy to audit in pull requests.

**Dependencies and scanning.** We run static analysis and vulnerability scanning on the codebase and dependencies (see [Static analysis](#static-analysis-sast) below).

**Releases.** Tagged releases ship with checksums so you can confirm files were not corrupted in transit. We also attach build provenance (cryptographic attestations) so you can verify that a binary was produced by our official GitHub release workflow, not replaced afterward.

**Supply chain.** CI and tagged-release publishing run on **GitHub Actions**, with workflow definitions in `.github/workflows/` on GitHub-hosted runners. That is what enables Sigstore-backed **artifact attestations** and the **SLSA Build Level 3**-oriented provenance model [GitHub documents](https://docs.github.com/en/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds) for those builds (workflows were moved here from `.gitea/workflows/` for that reason). Tool versions in CI are pinned where practical; GitHub’s own actions are referenced by full commit SHA. Optional cosign-based verification for separately produced bundles is documented below.

The sections below spell out the same points with paths, tools, and verification steps for technical readers.

## Supply chain and CI

**In short.** All automation paths described here use `.github/workflows/` on GitHub Actions. Jobs call installers and helpers in `scripts/ci/*.sh` (Go, Task, Node, cosign, gosec, govulncheck, Trivy, revive, Python venv, TinyGo, and similar) using pinned versions declared in workflow `env` blocks, then run `task` or `go` as appropriate. That layout keeps install and test logic in ordinary shell that you can diff like any other project code.

**Actions pinning.** GitHub-owned steps such as checkout, artifact upload/download, Node setup, attest, and related actions are pinned to full commit SHAs in the YAML (each workflow file notes this in a comment at the top).

**Bill of materials.** Software bill of materials (SBOM) generation uses CycloneDX; see `task sbom` and `.github/workflows/sbom.yml`.

**Reproducibility.** CI includes a reproducibility check (`task reproducibility`, `.github/workflows/reproducibility.yml`).

### Release provenance

Tagged releases are built and published from `.github/workflows/publish.yml` on GitHub Actions. The workflow’s `release` job runs only for `refs/tags/*` (not for ordinary branch pushes). Every published release includes SHA256 checksum sidecars next to the binaries.

After all matrix builds finish, that same workflow run [attests](https://docs.github.com/en/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds) the release binaries using GitHub’s [`actions/attest`](https://github.com/actions/attest) (Sigstore-backed provenance in the form GitHub associates with **SLSA Build Level 3** expectations), then creates the GitHub release in a single `gh release create` step so assets are not appended piecemeal by later jobs.

**Verifying attestations.** With a recent GitHub CLI that supports attestation commands, you can verify a downloaded binary against the repository, for example:

`gh attestation verify PATH/TO/reticulum-go-linux-amd64 --repo OWNER/REPO`

**Cosign bundles.** cosign bundles (`*.cosign.bundle`) from `scripts/ci/attest-release-assets.sh` are not produced by the default GitHub publish path. If bundles are attached separately, verification uses the public key at `cosign.pub` and `sh scripts/ci/verify-release-attestation.sh <blob> <bundle>` as documented in that script.

### Static analysis (SAST)

CI runs **Gosec** (Go security linter), **govulncheck** (official Go vulnerability database with reachable-code analysis for `go.mod` dependencies), and **Trivy** (filesystem and dependency scanning) in `.github/workflows/scan.yml`.

- Gosec is installed via `scripts/ci/setup-gosec.sh` with a pinned module version.
- Govulncheck is installed via `scripts/ci/setup-govulncheck.sh` with a pinned module version.
- Trivy is not installed from moving GitHub Action tags or unverified release URLs in the workflow. The job downloads a pinned `.deb` from a URL we control (`TRIVY_DEB_URL` in that workflow), checks it with `TRIVY_DEB_SHA256`, and installs through `scripts/ci/setup-trivy.sh`. We bump the URL and hash deliberately when upgrading Trivy.

**Why Trivy is pinned this way.** Third-party distribution channels are a common supply-chain risk. For example, in March 2026 attackers compromised parts of the Trivy ecosystem by repointing GitHub Action tags and distributing trojanized binaries through plausible official paths; workflows that followed moving tags or unverified binaries could have run malicious code in CI or on developer machines. Hosting a known-good package at an immutable URL with a recorded SHA256 avoids depending on those surfaces for our scans.

## Cryptography in this project

Reticulum-Go uses standard Go cryptography APIs and a small set of supplementary packages from the Go extended library. Below is a concise inventory for reviewers; see `go.mod` and `pkg/cryptography/` for authoritative usage.

**Standard library (`crypto/...`, `crypto/*` via Go)**

- ed25519, sha256, rand, aes, cipher, hmac

**Extended library**

- `golang.org/x/crypto` at `v0.50.0` (as in `go.mod`): HKDF, curve25519
