# Security policy

This document explains how we think about security for Reticulum-Go, how to report problems, and supply-chain and CI practices. For cryptography primitives, key formats, and protocol-aligned behavior, see [docs/cryptography.md](docs/cryptography.md).

## Reporting a vulnerability

If you believe you have found a security issue, please tell us privately first so we can investigate before wider disclosure.

**How to reach us**

- Reticulum LXMF: `7cc8d66b4f6a0e0e49d34af7f6077b5a`
- Email: `security@quad4.io`

Include enough detail to reproduce or understand the issue (what component, what you expected, what happened). We will treat reports seriously and work with you on a sensible timeline for fixes and public communication.

## What we do in practice (overview)

**Builds and automation.** Continuous integration runs tests and security-oriented checks on every change. We prefer clear, reviewable shell scripts over long chains of opaque third-party actions so that what runs in CI is easy to audit in pull requests.

**Dependencies and scanning.** We run static analysis and vulnerability scanning on the codebase and dependencies (see [Static analysis](#static-analysis-sast) below).

**Releases.** Tagged releases attach **cosign** provenance bundles (`*.cosign.bundle`) next to each asset, signed with a project key (public half in `cosign.pub`). Informal SHA256 digests are listed in the GitHub release notes as a backup only; prefer cosign verification.

**Supply chain.** CI and tagged-release publishing run on **GitHub Actions**, with workflow definitions in `.github/workflows/` on GitHub-hosted runners (moved from `.gitea/workflows/`). Release provenance uses **cosign** blob attestations (`scripts/ci/attest-release-assets.sh`) with a SLSA-style predicate (`scripts/ci/slsa-predicate.py`). Tool versions in CI are pinned where practical; GitHub’s own actions are referenced by full commit SHA where applicable.

The sections below spell out the same points with paths, tools, and verification steps for technical readers.

## Supply chain and CI

**In short.** All automation paths described here use `.github/workflows/` on GitHub Actions. Jobs call installers and helpers in `scripts/ci/*.sh` (Go, Task, Node, cosign, gosec, govulncheck, Trivy, revive, Python venv, TinyGo, and similar) using pinned versions declared in workflow `env` blocks, then run `task` or `go` as appropriate. That layout keeps install and test logic in ordinary shell that you can diff like any other project code.

**Actions pinning.** GitHub-owned steps such as checkout, artifact upload/download, Node setup, and related actions are pinned to full commit SHAs in the YAML where this repository pins them (see the comment at the top of each workflow file).

**Bill of materials.** SPDX and CycloneDX SBOMs are produced with Trivy (`task sbom`); tagged releases attach them from `.github/workflows/publish.yml`. The standalone `workflow_dispatch` workflow `.github/workflows/sbom.yml` is available for ad-hoc generation.

**Reproducibility.** CI includes a reproducibility check (`task reproducibility`, `.github/workflows/reproducibility.yml`).

### Release provenance

Tagged releases are built and published from `.github/workflows/publish.yml` on GitHub Actions. The `release` job runs only for `refs/tags/*` (not for ordinary branch pushes). The same workflow builds main binaries, **wasm** and **pageserver** examples, and SBOMs (SPDX and CycloneDX), uploads them as workflow artifacts, then runs **cosign** (`scripts/ci/setup-cosign.sh` and `scripts/ci/attest-release-assets.sh`) to emit one `*.cosign.bundle` per file. There are no separate `.sha256` sidecar files; a `sha256sum` listing is appended to the auto-generated release notes as an informal backup.

**Verifying cosign bundles.** Use the committed public key `cosign.pub` and `sh scripts/ci/verify-release-attestation.sh PATH/TO/blob PATH/TO/blob.cosign.bundle`, or `cosign verify-blob-attestation` with the same key and bundle paths as in that script.

### Static analysis (SAST)

CI runs **Gosec** (Go security linter), **govulncheck** (official Go vulnerability database with reachable-code analysis for `go.mod` dependencies), and **Trivy** (filesystem and dependency scanning) in `.github/workflows/scan.yml`.

- Gosec is installed via `scripts/ci/setup-gosec.sh` with a pinned module version.
- Govulncheck is installed via `scripts/ci/setup-govulncheck.sh` with a pinned module version.
- Trivy is not installed from moving GitHub Action tags or unverified release URLs in the workflow. The job downloads a pinned `.deb` from a URL we control (`TRIVY_DEB_URL` in that workflow), checks it with `TRIVY_DEB_SHA256`, and installs through `scripts/ci/setup-trivy.sh`. We bump the URL and hash deliberately when upgrading Trivy.

**Why Trivy is pinned this way.** Third-party distribution channels are a common supply-chain risk. For example, in March 2026 attackers compromised parts of the Trivy ecosystem by repointing GitHub Action tags and distributing trojanized binaries through plausible official paths; workflows that followed moving tags or unverified binaries could have run malicious code in CI or on developer machines. Hosting a known-good package at an immutable URL with a recorded SHA256 avoids depending on those surfaces for our scans.

## Cryptography

Algorithms, key formats, identity encryption, IFAC, links, ratchets, operational handling, and verification pointers are documented in **[docs/cryptography.md](docs/cryptography.md)**. That file is the canonical cryptography reference for this repository; this `SECURITY.md` focuses on reporting, supply chain, and automation.
