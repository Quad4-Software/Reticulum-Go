// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build rns_slim

package node

import (
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/discovery"
)

func (n *Node) autoconnectI2P(_ *discovery.ReceivedAnnounceInfo, _ string, _ []byte, _ *common.InterfaceConfig) bool {
	debug.Log(debug.DebugWarning, "Auto-connecting discovered I2P interfaces requires a full build without rns_slim")
	return true
}
