// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package identity

import "errors"

// ErrSigningMaterialNotExportable is returned when the identity uses an
// external signer (e.g. HSM) and raw private bytes are not available.
var ErrSigningMaterialNotExportable = errors.New("identity signing material is not exportable")
