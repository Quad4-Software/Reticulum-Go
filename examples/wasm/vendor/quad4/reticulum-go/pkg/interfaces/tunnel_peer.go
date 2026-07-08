// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import "quad4/reticulum-go/pkg/common"

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
