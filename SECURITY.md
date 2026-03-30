# Security Policy

## Supply Chain Security

- All actions are pinned to a full-length commit hash and have been forked to my Gitea instance in https://git.quad4.io/actions
- BOM generation using CycloneDX (see `task sbom` and `.gitea/workflows/sbom.yml`)
- Release binaries include SHA256 sidecars from CI; tagged releases can also ship **SLSA v1** provenance as **cosign** bundles (`*.cosign.bundle`) next to each artifact when the repository secret `COSIGN_PRIVATE_KEY` is set (PEM from `cosign generate-key-pair`). Commit the matching `cosign.pub` in the repo root so anyone can verify offline without the transparency log (`--private-infrastructure` matches self-hosted runners that do not upload to Rekor).
- Reproducibility is checked in CI (`task reproducibility`, `.gitea/workflows/reproducibility.yml`).

### Verifying a release attestation

With `cosign` installed and `cosign.pub` from this repository:

`sh scripts/ci/verify-release-attestation.sh path/to/reticulum-go-linux-amd64 path/to/reticulum-go-linux-amd64.cosign.bundle`

## Cryptography Dependencies

- golang.org/x/crypto `v0.48.0` for core cryptographic primitives
  - hkdf
  - curve25519

- go/crypto
  - ed25519
  - sha256
  - rand
  - aes
  - cipher
  - hmac

## Reporting a Vulnerability

Refer to [https://quad4.io/security](https://quad4.io/security) for how to report vulnerabilities.