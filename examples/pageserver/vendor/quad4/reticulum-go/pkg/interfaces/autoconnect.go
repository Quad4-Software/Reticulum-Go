// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
)

// I2PSpawnedLister exposes outbound I2P peers owned by a parent interface.
type I2PSpawnedLister interface {
	ListSpawnedPeers() []Interface
}

// MatchesDiscoveredEndpoint reports whether iface already covers a discovered
// endpoint (Python InterfaceDiscovery.interface_exists).
func MatchesDiscoveredEndpoint(iface Interface, endpointHash []byte, reachableOn string, port int64, hasPort bool) bool {
	if iface == nil {
		return false
	}
	switch v := iface.(type) {
	case *BackboneClientInterface:
		if len(endpointHash) > 0 && len(v.AutoconnectHash) > 0 && bytes.Equal(v.AutoconnectHash, endpointHash) {
			return true
		}
		if reachableOn != "" && v.TargetHost() == reachableOn {
			if !hasPort || v.TargetPort() == int(port) {
				return true
			}
		}
	case *TCPClientInterface:
		if len(endpointHash) > 0 && len(v.AutoconnectHash) > 0 && bytes.Equal(v.AutoconnectHash, endpointHash) {
			return true
		}
		if reachableOn != "" && v.TargetHost() == reachableOn {
			if !hasPort || v.TargetPort() == int(port) {
				return true
			}
		}
	}
	return matchesDiscoveredEndpointI2P(iface, endpointHash, reachableOn)
}
