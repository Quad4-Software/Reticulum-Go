// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build rns_slim

package interfaces

func matchesDiscoveredEndpointI2P(_ Interface, _ []byte, _ string) bool {
	return false
}
