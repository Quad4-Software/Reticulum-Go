# Security Policy

## Supply Chain Security

- All actions are pinned to a full-length commit hash and have been forked to my Gitea instance in https://git.quad4.io/actions
- BOM generation using CycloneDX

## Cryptography Dependencies

- golang.org/x/crypto `v0.46.0` for core cryptographic primitives
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