// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package librns

const (
	// APIVersion is the librns C ABI version (major.minor).
	APIVersion = "1.6"
)

// Version returns the librns API version string.
func Version() string {
	return APIVersion
}
