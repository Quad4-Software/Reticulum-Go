// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package cryptography is the single integration point for cryptographic
// primitives used across Reticulum-Go. Application and library code should
// call the exported functions here (or types derived from them) rather than
// importing lower-level packages such as crypto/ed25519 or curve25519 directly,
// so algorithms and test doubles can be changed in one place.
//
// Extension model:
//
//   - [CryptoProvider] defines the full surface; [stdlibProvider] is the default.
//   - [SetProvider] installs a replacement (for tests or future algorithms);
//     [SetProvider](nil) restores the default.
//   - [ActiveProvider] returns the current implementation.
//
// On-wire formats (key sizes, packet layouts, hash truncation) are defined
// elsewhere and assume the default provider’s behavior. Replacing the provider
// without updating those formats will break interoperability; treat provider
// swaps as coordinated protocol changes unless you control all peers.
//
// Hardware signing (HSM, PKCS#11, cloud KMS) can integrate via [Ed25519Signer]:
// use [NewSoftwareEd25519Signer] for in-memory seeds, or [NewEd25519SignerFromCryptoSigner]
// to wrap a standard [crypto.Signer] that performs Ed25519. Identity wiring is
// in package identity ([git.quad4.io/Networks/Reticulum-Go/pkg/identity.NewIdentityWithSigner]).
package cryptography
