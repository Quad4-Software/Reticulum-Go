// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

const (
	// APIVersion is the librns C ABI version (major.minor).
	APIVersion = "1.4"
)

// Version returns the librns API version string.
func Version() string {
	return APIVersion
}
