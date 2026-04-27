# Cryptography in Reticulum-Go

This document is the canonical cryptography description for security reviews and operators. For vulnerability reporting and supply-chain practices, see [SECURITY.md](../SECURITY.md) in the repository root.

Implementation code lives primarily in `pkg/cryptography`, `pkg/identity`, `pkg/ifac`, and `pkg/link`. On-wire layouts and numeric constants are aligned with the Python reference implementation; changing algorithms or sizes without updating both stacks breaks interoperability.

## Design goals

- **Interoperability**: Peers running the official Python stack must verify signatures, decrypt identity-encrypted payloads (when keys match), and participate in links and transport modes supported by both implementations.
- **Single integration surface**: Call exported APIs in `pkg/cryptography` (and identity helpers in `pkg/identity`) so primitives, test doubles, and future `CryptoProvider` swaps stay in one place.
- **Explicit non-goals**: This repository does not implement post-quantum algorithms, alternative curves for Reticulum identities, or custom TLS-style handshakes outside what the Reticulum protocol specifies.

## Primitive inventory

| Primitive | Role | Implementation notes |
|-----------|------|----------------------|
| X25519 (Curve25519 ECDH) | Static identity DH key; ephemeral keys for identity encrypt; ratchets; IFAC key derivation input | `golang.org/x/crypto/curve25519` via `pkg/cryptography` |
| Ed25519 | Identity signatures (announces, proofs, related auth paths); IFAC inner identity | `crypto/ed25519` via `pkg/cryptography` |
| SHA-256 | Hashing for identity hash truncation, destination construction, link and packet digests as defined by the protocol | `crypto/sha256` |
| HKDF-SHA256 | Identity encrypt key material (RFC 5869); IFAC key derivation from netname/passphrase | `golang.org/x/crypto/hkdf` |
| AES-256-CBC | Symmetric encryption for identity-layer tokens and link traffic where the reference uses CBC | `crypto/aes`, `crypto/cipher`; PKCS#7 padding in `pkg/cryptography` |
| HMAC-SHA256 | Authenticity for identity ciphertext tokens | `crypto/hmac` |
| Random bytes | Key generation, IVs where applicable | `crypto/rand` |

**Go modules (pinned in `go.mod`)**

- Standard library: `crypto/ed25519`, `crypto/sha256`, `crypto/rand`, `crypto/aes`, `crypto/cipher`, `crypto/hmac`, and related types.
- Extended: `golang.org/x/crypto` (HKDF, curve25519), version as locked in `go.mod` (currently `v0.50.0`).

## Identity keys and public fingerprint

- **Key material**: A Reticulum identity uses a **256-bit X25519 private scalar** (encryption / ECDH) and a **256-bit Ed25519 seed** (signing), matching the reference “512-bit keyset” wording.
- **Public form (64 bytes on the wire)**: Concatenation of **X25519 public key (32)** || **Ed25519 public key (32)**. This blob is what peers learn from announces and proofs.
- **Identity hash**: SHA-256 over the 64-byte public key, truncated to **128 bits** (16 bytes). It is used as salt/context input for identity-layer HKDF and for addressing logic tied to identity.
- **Software on-disk file (64 bytes)**: Raw `X25519_private (32) || Ed25519_seed (32)`. Equivalent layout to Python `RNS.Identity` persistence for that path. Protecting this file is equivalent to protecting the full identity.
- **Hardware-bound descriptor (optional, Reticulum-Go, 72 bytes)**: Magic `RHB1`, version byte, reserved, **X25519 private (32)**, **Ed25519 public (32)** only. Signing uses `cryptography.Ed25519Signer` (for example `crypto.Signer` from PKCS#11). On-wire public keys stay identical to software identities so Python peers do not need changes. Python `Identity.from_file` does not yet read this layout; see [COMPATIBILITY.md](../COMPATIBILITY.md).

## Identity encryption (outbound “token”)

When encrypting to another identity’s public X25519 key:

1. Generate an **ephemeral X25519** keypair.
2. ECDH: ephemeral private with peer’s **public encryption key** (or an optional X25519 ratchet public key when ratchets are in use).
3. **HKDF-SHA256** (RFC 5869) expands the shared secret to **64 bytes** of key material: first 32 bytes **HMAC key**, next 32 bytes **AES-256 key**. Salt and info are driven by the identity’s hash and protocol context as implemented in `DeriveIdentityKeyMaterial` / `Identity` methods (see `pkg/cryptography/identity_hkdf.go` and `pkg/identity`).
4. **AES-256-CBC** encrypts the plaintext with PKCS#7 padding.
5. **HMAC-SHA256** over the ciphertext authenticates the token.
6. On the wire, the token includes the **ephemeral X25519 public key** followed by ciphertext and MAC (exact layout in `pkg/identity`).

Decryption reverses the steps using the recipient’s private X25519 key and verifies HMAC before decrypting.

## Signing, announces, and proofs

- **Algorithm**: Ed25519 (pure EdDSA over Curve25519 in the usual Ed25519 parametrization).
- **Usage**: Signing packet hashes and announce payloads as required by the reference announce and proof paths (`pkg/identity`, `pkg/announce`, transport handlers).
- **Hardware and HSMs**: `Ed25519Signer` and `NewEd25519SignerFromCryptoSigner` allow signing without holding the Ed25519 seed in process memory; the **public** Ed25519 key must still match the 64-byte public blob announced to the network. Integrations must still perform Ed25519 over the exact bytes the protocol specifies.

## Destination hashes (summary)

Destination hashes used on the wire are derived from the **application name and aspects** plus the **identity hash** (for non-plain destinations): a truncated SHA-256 construction documented in `pkg/destination` and aligned with the reference. This document does not duplicate every byte offset; reviewers should read `calculateHash` and the reference manual for bit-exact parity.

## Links and resources

- **Links** (`pkg/link`) establish confidential channels to destinations using the same cryptographic suite the reference enables for link mode (including **AES-256-CBC** for the enabled link path). Session keys, KDF steps, and packet contexts follow the ported logic; resource transfers reuse the same stack for encryption and integrity where applicable.
- **Details**: See `pkg/link` and cross-reference tests; do not assume TLS or AEAD unless a future coordinated protocol change adds them.

## Interface Access Code (IFAC)

IFAC is an **optional outer authentication layer** on interface frames (UDP, TCP, Auto, etc.):

- A fixed **HKDF salt** (`pkg/ifac`, constant `SaltHex`) and operator-supplied **network name / passphrase** derive key material.
- Derived material includes an inner **Reticulum identity** used to sign a truncated tail of the Ed25519 signature over the frame, producing the on-wire IFAC tag size for that interface.
- **Policy**: If an interface is configured with IFAC, outbound paths mask packets and inbound paths unmask and verify; packets with wrong or missing IFAC are dropped (`pkg/common.ApplyIFACOutbound` / `ApplyIFACInbound`).

## Ratchets

- Optional **X25519 ratchet** private keys (256 bits) can be rotated for forward secrecy on identity-encrypted traffic where the protocol uses them.
- Ratchet state can be persisted per identity hash (`pkg/identity`); expiry and retention limits follow reference-oriented constants in that package.

## Pluggable `CryptoProvider`

`pkg/cryptography` supports replacing the active provider (`SetProvider`) for tests or experiments. **Replacing algorithms or key formats without updating on-wire packet layouts breaks compatibility** with Python peers and with stored artifacts. Treat provider swaps as protocol forks unless all participants upgrade together.

## Operational storage and handling

- Identity and transport identity files should live on encrypted disks where possible; Unix permissions are written restrictively by the implementations that create them (see code paths in `cmd/reticulum-go` and `pkg/identity`).
- **Backup and disclosure**: Anyone with the 64-byte software identity file or the combination of RHB1 descriptor + signing capability can impersonate the identity on the network.
- **Logs**: Debug logging can include hex dumps; disable verbose logging in production if metadata sensitivity matters.

## Verification

- Automated cross-tests: `tests/crossref` and package tests under `pkg/*` and `tests/interop`.
- External spec: [Reticulum manual](https://reticulum.network/manual/reference.html) and [crypto overview](https://reticulum.network/crypto.html) for the reference stack’s intent; this Go tree is tested for parity where claimed in [COMPATIBILITY.md](../COMPATIBILITY.md).
