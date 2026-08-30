# Identity and destinations

## Identity

A Reticulum identity is the root cryptographic principal for signing announces, establishing links, and encrypting identity-layer messages.

Implementation: pkg/identity with primitives from pkg/cryptography.

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

Equivalent to Python RNS.Identity software persistence. Anyone with this file can impersonate the identity on the network. Store with restrictive permissions on encrypted media.

Load helpers:

```go
id, err := identity.LoadIdentityFile(path, nil)
```

### Identity backend (file, Secret Service, or kernel keyring)

Config key identity_backend in [reticulum]:

| Value | Behavior |
|-------|----------|
| file (default) | 64-byte plaintext identity files |
| secretservice | Freedesktop Secret Service over D-Bus (GNOME Keyring, KDE Wallet Secret Service bridge, KeePassXC with Secret Service enabled) |
| keyring | Linux kernel keyring (user keyring, no D-Bus). Suitable for headless systemd user or system units |

With secretservice or keyring, ToFile stores the private blob in the backend and writes an 8-byte RSSI marker at the usual path. FromFile and LoadIdentityFile detect the marker and fetch the secret. If the backend is unavailable, persistence fails with a clear error (no silent plaintext fallback).

keyring is Linux-only. When a persistent user keyring is available, material can survive reboot for the same UID. Otherwise keys are cleared on reboot. Desktop sessions without D-Bus can use keyring instead of secretservice.

Headless units that cannot use the kernel keyring should keep identity_backend = file, or unlock a Secret Service collection / KeePassXC before start when using secretservice.

CLI migrate (path via -i):

```
rgoid -i ~/.reticulum-go/storage/transport_identity -to-secretservice
rgoid -i ~/.reticulum-go/storage/transport_identity -to-keyring
rgoid -i ~/.reticulum-go/storage/transport_identity -to-file
```

### In-memory key handling

Long-term X25519 and Ed25519 material lives in pkg/securemem buffers with best-effort mlock and wipe on Identity.Close. Callers of GetPrivateKey should wipe the returned slice when finished.

### What this protects against

| Control | Helps against | Does not stop |
|---------|---------------|---------------|
| identity_backend = secretservice | Casual theft of the identity file path (backup copies, world-readable home dirs, malware that only reads ~/.reticulum-go/storage). Keyring unlock policies and KeePassXC master password raise the bar for offline disk images when the collection is locked. | Root or same-user malware that can talk to an unlocked Secret Service session. Compelled unlock. Physical access while the keyring is unlocked. |
| identity_backend = keyring | Same file-path theft cases without requiring D-Bus. Fits systemd units that have a user keyring. | Root or same-UID processes that can call keyctl. Keys may not survive reboot if persistent keyring is unavailable. |
| pkg/securemem mlock + wipe | Keys lingering in swap after process exit, casual core dumps of freed heap (with RLIMIT_CORE=0 in the sandbox), accidental retention after Close. | Live process memory inspection by root or a debugger attached to the running daemon. Full cold-boot attacks on RAM. |
| File permissions 0600 + encrypted disk (operator practice) | Other local users reading plaintext identity files. Disk theft when FDE is used and the volume is locked. | Attacks after the volume is unlocked and mounted. |

None of these replace HSM-backed signing (RHB1 / NewIdentityWithSigner) for high-assurance signing material, or host firewall and sandbox policy for network exposure.

### Hardware-bound descriptor (RHB1, optional)

Reticulum-Go supports a 72-byte hardware-bound layout:

| Field | Size |
|-------|------|
| Magic RHB1 | 4 bytes |
| Version | 1 byte |
| Reserved | 3 bytes |
| X25519 private | 32 bytes |
| Ed25519 public | 32 bytes |

Signing uses an external cryptography.Ed25519Signer (for example PKCS#11 or cloud HSM). The on-wire Ed25519 public key matches a software identity so Python peers require no changes.

Python Identity.from_file does not read RHB1 today. See [Compatibility](compatibility.md).

```go
id, err := identity.NewIdentityWithSigner(descriptor, signer)
```

### Identity encryption

Outbound encrypted tokens use ephemeral X25519, HKDF-SHA256, AES-256-CBC, and HMAC-SHA256. See [Cryptography](cryptography.md#identity-encryption).

### Ratchets

Identity ratchets are optional, same as Python Destination.enable_ratchets. They are off until the destination calls EnableRatchets or EnableRatchetsInMemory. EnforceRatchets is a separate opt-in, same as Python enforce_ratchets. Python RNS does not enable or enforce ratchets by default.

When enabled, the destination rotates X25519 ratchet keys and puts the current 32-byte public key in announces (header context flag set). Peers call Identity.RememberRatchet keyed by destination hash. Destination.Encrypt for SINGLE destinations uses Identity.GetRatchet(destHash) when a non-expired key is known, otherwise the static identity encryption key.

Two kinds of material share the ratchet directory:

| Kind | Path | Contents |
|------|------|----------|
| Local destination private keys | Path passed to EnableRatchets (pageserver uses storage/ratchets/{destination_hash}) | Signed msgpack list (signature + packed private keys) |
| Known-peer public keys | storage/ratchets/{destination_hash} | Python-compatible msgpack {ratchet, received} |

EnableRatchetsInMemory keeps local private keys in RAM only. Known-peer public keys also stay in RAM when in_memory_storage or shared-instance client mode is on (Identity.RememberRatchet skips disk). Expired known-peer files are dropped after 30 days (RATCHET_EXPIRY). Clean skips destination-private signed files so they are not treated as peer records.

EnforceRatchets refuses ciphertext that still uses the static identity key. Links do not use identity ratchets. They already exchange ephemeral keys at establishment.

GROUP destinations use a pre-shared Token key, not identity ratchets. Call CreateKeys or LoadPrivateKey with the 64-byte AES-256 Token key (or a 32-byte AES-128 key). Encrypt / Decrypt match Python Token.encrypt / Token.decrypt (IV || ciphertext || HMAC-SHA256). GROUP dests cannot announce. Outbound GROUP packets are broadcast on local interfaces and are dropped after one hop, same as Python.

## Destinations

A destination is an application endpoint on the network. Implementation: pkg/destination.

### Destination types

| Type | Constant | Use |
|------|----------|-----|
| Single | SINGLE | One-to-one application endpoint |
| Group | GROUP | Shared-key messaging (Token PSK, not identity ratchets) |
| Plain | PLAIN | Unencrypted endpoint (rare) |
| Link | LINK | Link-mode endpoint |

### Destination hash

The on-wire destination hash is derived from application name, aspects, and identity hash (for non-plain types) using a truncated SHA-256 construction in calculateHash. The exact byte layout matches Python and is verified in tests/crossref.

### Creating a destination

```go
dest, err := destination.New(
    id,
    destination.In,
    destination.Single,
    "myapp",
    tr,
    "subsystem",
)
```

Register with transport so inbound packets route to Destination.Receive.

### Announces

Destinations publish signed announces so the network learns paths. Handlers register through pkg/announce:

```go
transport.RegisterAnnounceHandler(handler)
```

Announce payloads can carry application-specific app_data visible to peers.

### Request handlers

Destinations can register request paths. Incoming requests block the link goroutine until the handler responds or a timeout elapses. The control API exposes the same pattern over WebSocket events.

### Incoming links

Register with accepts_links semantics. Incoming link requests dispatch to link.HandleIncomingLinkRequest.

## Resolver

pkg/resolver resolves a human-readable full name string to a deterministic identity hash via SHA-256. Useful for configuration and display, not a substitute for trust on first use from announces.

## Storage layout

| Artifact | Path |
|----------|------|
| Identity blobs | storage/identities/ (per hash in Go) |
| Known destinations | storage/known_destinations/ |
| Known-peer ratchet public keys | storage/ratchets/{destination_hash} |
| Local destination ratchet private keys | Path from EnableRatchets (often under storage/ratchets/) |

Go writes identity files keyed by hash. Python may use per-name files. Go writes and loads Python-compatible known destination tables (16-byte keys) and still loads legacy Go hex-keyed files.

## Signing without holding the seed

Integrate HSM or cloud KMS via cryptography.NewEd25519SignerFromCryptoSigner. The public Ed25519 key in announces must match the 64-byte public blob.

## Security practices

- Treat the 64-byte software file like a private key backup
- Use RHB1 when the signing key must not live in process memory
- Do not log identity material at high debug levels in production
- Rotate ratchets according to application policy and storage expiry

## Related documents

- [API reference](api-reference.md)
- [Cryptography](cryptography.md)
- [Transport](transport.md) for announce routing
- [Links, channels, and resources](links-channels-and-resources.md) for link-mode destinations
