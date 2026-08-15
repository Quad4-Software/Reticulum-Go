// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build tinygo

package backbone

// DefaultBackend always uses the portable Go poller on TinyGo.
func DefaultBackend() Backend {
	return BackendGo
}
