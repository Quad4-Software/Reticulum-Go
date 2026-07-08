# Identity and destinations

| Field | Value |
|-------|-------|
| Document version | 1.0 |
| Last updated | 2026-07-07 |
| Author | Ivan |

## Identity

A Reticulum identity is the root cryptographic principal for signing announces, establishing links, and encrypting identity-layer messages.

Implementation: `pkg/identity` with primitives from `pkg/cryptography`.

### Key material

| Component | Size | Purpose |
|-----------|------|---------|
| X25519 private scalar | 32 bytes | ECDH, identity encrypt |
| Ed25519 seed | 32 bytes | Signing |

Together these form the 512-bit keyset described in Reticulum documentation.

### Public form (64 bytes on wire)

```
X25519 public key (32 bytes) || Ed25519 public key (32 bytes)
```

Peers learn this blob from announces and proofs.

### Identity hash

SHA-256 over the 64-byte public key, truncated to 128 bits (16 bytes). Used for addressing, HKDF salt context, and storage paths.

### Software identity file (64 bytes on disk)

```
X25519 private (32) || Ed25519 seed (32)
```

Equivalent to Python `RNS.Identity` software persistence. Anyone with this file can impersonate the identity on the network. Store with restrictive permissions on encrypted media.

Load helpers:

```go
id, err := identity.LoadIdentityFile(path)
```

### Hardware-bound descriptor (RHB1, optional)

Reticulum-Go supports a 72-byte hardware-bound layout:

| Field | Size |
|-------|------|
| Magic `RHB1` | 4 bytes |
| Version | 1 byte |
| Reserved | 3 bytes |
| X25519 private | 32 bytes |
| Ed25519 public | 32 bytes |

Signing uses an external `cryptography.Ed25519Signer` (for example PKCS#11 or cloud HSM). The on-wire Ed25519 public key matches a software identity so Python peers require no changes.

Python `Identity.from_file` does not read RHB1 today. See [Compatibility](compatibility.md).

```go
id, err := identity.NewIdentityWithSigner(descriptor, signer)
```

### Identity encryption

Outbound encrypted tokens use ephemeral X25519, HKDF-SHA256, AES-256-CBC, and HMAC-SHA256. See [Cryptography](cryptography.md#identity-encryption).

### Ratchets

Optional X25519 ratchet keys rotate for forward secrecy on identity-encrypted traffic. Ratchets persist under `storage/ratchets/` per identity hash with expiry aligned to reference constants.

## Destinations

A destination is an application endpoint on the network. Implementation: `pkg/destination`.

### Destination types

| Type | Constant | Use |
|------|----------|-----|
| Single | SINGLE | One-to-one application endpoint |
| Group | GROUP | Group messaging |
| Plain | PLAIN | Unencrypted endpoint (rare) |
| Link | LINK | Link-mode endpoint |

### Destination hash

The on-wire destination hash is derived from application name, aspects, and identity hash (for non-plain types) using a truncated SHA-256 construction in `calculateHash`. The exact byte layout matches Python and is verified in `tests/crossref`.

### Creating a destination

```go
dest, err := destination.NewDestination(
    id,
    destination.TypeSingle,
    "myapp",
    "subsystem",
)
```

Register with transport so inbound packets route to `Destination.Receive`.

### Announces

Destinations publish signed announces so the network learns paths. Handlers register through `pkg/announce`:

```go
transport.RegisterAnnounceHandler(handler)
```

Announce payloads can carry application-specific `app_data` visible to peers.

### Request handlers

Destinations can register request paths. Incoming requests block the link goroutine until the handler responds or a timeout elapses. The control API exposes the same pattern over WebSocket events.

### Incoming links

Register with `accepts_links` semantics. Incoming link requests dispatch to `link.HandleIncomingLinkRequest`.

## Resolver

`pkg/resolver` resolves a human-readable full name string to a deterministic identity hash via SHA-256. Useful for configuration and display, not a substitute for trust on first use from announces.

## Storage layout

| Artifact | Path |
|----------|------|
| Identity blobs | `storage/identities/` (per hash in Go) |
| Known destinations | `storage/known_destinations/` |
| Ratchets | `storage/ratchets/` |

Go writes identity files keyed by hash. Python may use per-name files. Go loads Python-format known destination files for interoperability.

## Signing without holding the seed

Integrate HSM or cloud KMS via `cryptography.NewEd25519SignerFromCryptoSigner`. The public Ed25519 key in announces must match the 64-byte public blob.

## Security practices

- Treat the 64-byte software file like a private key backup
- Use RHB1 when the signing key must not live in process memory
- Do not log identity material at high debug levels in production
- Rotate ratchets according to application policy and storage expiry

## Related documents

- [Cryptography](cryptography.md)
- [Transport](transport.md) for announce routing
- [Links, channels, and resources](links-channels-and-resources.md) for link-mode destinations
