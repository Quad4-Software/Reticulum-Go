// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
)

// FromConfigContext carries runtime dependencies for interface types that
// need storage paths, transport identity, or dynamic peer registration.
type FromConfigContext struct {
	I2PStoragePath        string
	TransportID           []byte
	RegisterPeer          func(name string, peer common.NetworkInterface) error
	UnregisterPeer        func(name string)
	SetupPeer             func(peer common.NetworkInterface)
	SynthesizeTunnel      func(TunnelPeer)
	VoidTunnel            func(TunnelPeer)
	WatchInterfaces       bool
	DiscoverInterfaces    bool
	PanicOnInterfaceError bool
	BackboneHub           *backbone.Hub
	SpawnBackbone         func(client *BackboneClientInterface)
	SpawnLocal            LocalSpawnHook
}
