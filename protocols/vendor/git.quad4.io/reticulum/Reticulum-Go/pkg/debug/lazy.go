// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package debug

import (
	"encoding/hex"
	"log/slog"
)

// Hex is a lazy hexadecimal representation for structured log fields.
// Converting a byte slice to Hex does not copy or allocate. The string is
// encoded only when the log record is emitted, unlike fmt.Sprintf("%x", b),
// which allocates before Log can filter the record.
type Hex []byte

// LogValue implements slog.LogValuer.
func (h Hex) LogValue() slog.Value {
	return slog.StringValue(hex.EncodeToString(h))
}

// String implements fmt.Stringer for non-slog use.
func (h Hex) String() string {
	return hex.EncodeToString(h)
}
