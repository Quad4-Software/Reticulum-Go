// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import "errors"

// ErrSigningMaterialNotExportable is returned when the identity uses an
// external signer (e.g. HSM) and raw private bytes are not available.
var ErrSigningMaterialNotExportable = errors.New("identity signing material is not exportable")

// ErrHardwareBoundSignerRequired means a RHB1 file was loaded without a signer
// and [OptionalIdentitySignerHook] did not supply one.
var ErrHardwareBoundSignerRequired = errors.New("hardware-bound identity file requires an Ed25519 signer or OptionalIdentitySignerHook")

// ErrHardwareBoundSignerPublicKeyMismatch means the signer public key != file pubkey.
var ErrHardwareBoundSignerPublicKeyMismatch = errors.New("signer public key does not match hardware-bound identity file")
