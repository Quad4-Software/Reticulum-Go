// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !rns_slim

package interfaces

import "bytes"

func matchesDiscoveredEndpointI2P(iface Interface, endpointHash []byte, reachableOn string) bool {
	peer, ok := iface.(*I2PInterfacePeer)
	if !ok {
		return false
	}
	if len(endpointHash) > 0 && len(peer.AutoconnectHash) > 0 && bytes.Equal(peer.AutoconnectHash, endpointHash) {
		return true
	}
	return reachableOn != "" && peer.TargetDest() == reachableOn
}
