// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build rns_slim

package node

import (
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/discovery"
)

func (n *Node) autoconnectI2P(_ *discovery.ReceivedAnnounceInfo, _ string, _ []byte, _ *common.InterfaceConfig) bool {
	debug.Log(debug.DebugWarning, "Auto-connecting discovered I2P interfaces requires a full build without rns_slim")
	return true
}
