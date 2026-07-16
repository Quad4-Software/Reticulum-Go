# Cryptography

## Purpose

This document is the canonical cryptography reference for Reticulum-Go. It supports security reviews and correct use of APIs. For vulnerability reporting and CI practices see [Security](security.md).

Implementation code:

- `pkg/cryptography` (primitives)
- `pkg/identity` (identity layer)
- `pkg/ifac` (interface access code)
- `pkg/link` (session crypto)

On-wire layouts match the Python reference. Changing algorithms or sizes without coordinated protocol updates breaks interoperability.

## Design goals

**Interoperability.** Peers on Python Reticulum must verify signatures, decrypt identity payloads when keys match, and complete link handshakes with Reticulum-Go nodes.

**Single integration surface.** Application code calls `pkg/cryptography` and `pkg/identity`. Do not import `crypto/ed25519` or curve25519 directly in application layers.

**Explicit non-goals.** This tree does not implement post-quantum algorithms, alternative curves for Reticulum identities, or custom TLS-style handshakes outside the Reticulum protocol.

## Primitive inventory

| Primitive | Role | Implementation |
|-----------|------|----------------|
| X25519 | Static identity DH, ephemeral ECDH, ratchets, IFAC input | `golang.org/x/crypto/curve25519` via `pkg/cryptography` |
| Ed25519 | Identity signatures, IFAC inner identity | `crypto/ed25519` via `pkg/cryptography` |
| SHA-256 | Hashes, destination construction, digests | `crypto/sha256` |
| HKDF-SHA256 | Identity encrypt keys, IFAC derivation | `golang.org/x/crypto/hkdf` |
| AES-256-CBC | Identity tokens, link traffic where reference uses CBC | `crypto/aes`, PKCS#7 in `pkg/cryptography` |
| HMAC-SHA256 | Identity ciphertext authentication | `crypto/hmac` |
| Random bytes | Key generation, IVs | `crypto/rand` |

Pinned extended module: `golang.org/x/crypto` (version in `go.mod`, currently v0.52.0).

## Identity keys

### Key material

- 256-bit X25519 private scalar (encryption / ECDH)
- 256-bit Ed25519 seed (signing)

### Public form (64 bytes)

```
X25519 public (32) || Ed25519 public (32)
```

### Identity hash

SHA-256 of the public blob, truncated to 128 bits (16 bytes).

### Software on-disk file (64 bytes)

```
X25519 private (32) || Ed25519 seed (32)
```

### Hardware-bound descriptor (RHB1, 72 bytes)

Optional Reticulum-Go format for external signing. Magic `RHB1`, version byte, reserved bytes, X25519 private, Ed25519 public only. On-wire public keys match software identities.

## Identity encryption

When encrypting to another identity public X25519 key:

1. Generate ephemeral X25519 keypair
2. ECDH with peer public encryption key (or optional ratchet public key)
3. HKDF-SHA256 expands shared secret to 64 bytes: HMAC key (32) then AES key (32). Salt and info follow identity hash and protocol context (`DeriveIdentityKeyMaterial` in `pkg/cryptography`)
4. AES-256-CBC encrypt with PKCS#7 padding
5. HMAC-SHA256 over ciphertext
6. Wire token includes ephemeral public key, ciphertext, and MAC

Decryption verifies HMAC before decrypting.

## Signing

Algorithm: Ed25519.

Used for announce signatures, proofs, and related auth paths.

Hardware signing via `Ed25519Signer` and `NewEd25519SignerFromCryptoSigner`. The announced Ed25519 public key must still match the 64-byte public blob.

## Destination hashes

Derived from application name, aspects, and identity hash (non-plain types) using truncated SHA-256 in `pkg/destination`. See crossref tests for byte-exact vectors.

## Links and resources

Links (`pkg/link`) use the same cryptographic suite as the reference for enabled link mode, including AES-256-CBC on the active link path. Session keys and KDF steps follow ported logic. Resources reuse link encryption where applicable.

Do not assume TLS or AEAD unless a future coordinated protocol change adds them.

## IFAC

Interface Access Code is an optional outer authentication layer on interface frames (UDP, TCP, Auto, and others).

Derivation:

- Fixed HKDF salt in `pkg/ifac` (`SaltHex`)
- Operator `network_name` and `passphrase`
- Inner Reticulum identity signs a truncated tail of the frame

Policy:

- Configured IFAC masks outbound frames and verifies inbound frames
- Wrong or missing IFAC is dropped (`ApplyIFACOutbound` / `ApplyIFACInbound` in `pkg/common`)

## Ratchets

Optional X25519 ratchet private keys (256 bits) for forward secrecy on identity-encrypted traffic. Persisted per identity hash under `storage/ratchets/` with expiry per package constants.

## Pluggable CryptoProvider

`cryptography.SetProvider` replaces the active provider for tests or experiments. Replacing algorithms without updating on-wire layouts breaks compatibility with Python peers and stored artifacts. Treat provider swaps as protocol forks unless all participants upgrade together.

## Operational handling

- Store identity files on encrypted disks with restrictive permissions
- Optional `identity_backend = secretservice` or `keyring` stores private blobs outside plaintext files
- Backup of the 64-byte software file or RHB1 plus signing capability equals full impersonation capability
- Verbose debug logging may hex-dump sensitive metadata. Lower loglevel in production

## Verification

| Method | Location |
|--------|----------|
| Automated cross tests | `tests/crossref`, package tests under `pkg/` |
| Live interop | `tests/interop` |
| External spec | [Reticulum manual](https://reticulum.network/manual/reference.html), [crypto overview](https://reticulum.network/crypto.html) |

Parity claims are listed in [Compatibility](compatibility.md).

## Protocol constants

| Item | Value |
|------|-------|
| Curve | Curve25519 (X25519 + Ed25519) |
| KEYSIZE | 512 bits (256 encryption + 256 signing) |
| TRUNCATED_HASHLENGTH | 128 bits |
| RATCHETSIZE | 256 bits |
| RATCHET_EXPIRY | 2 592 000 seconds (30 days) |
| Default MTU | 500 bytes (`pkg/packet.MTU`) |
