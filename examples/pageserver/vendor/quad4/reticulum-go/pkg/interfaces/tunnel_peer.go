// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/cryptography"
)

// TunnelPeer is implemented by interfaces that participate in Reticulum tunnel
// establishment (I2P peers, i2p_tunneled TCP clients).
type TunnelPeer interface {
	common.NetworkInterface
	InterfaceHash() []byte
	WantsTunnel() bool
	SetWantsTunnel(bool)
	TunnelID() []byte
	SetTunnelID([]byte)
}

// InterfaceConfigProvider supplies the parent interface config for spawned peers.
type InterfaceConfigProvider interface {
	InterfaceConfig() *common.InterfaceConfig
}

// InterfaceHashFromName returns the tunnel interface hash for a peer name.
func InterfaceHashFromName(name string) []byte {
	return cryptography.Hash([]byte("I2PInterfacePeer[" + name + "]"))
}
