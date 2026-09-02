// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !rns_slim

package node

import (
	"fmt"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/discovery"
	"quad4/reticulum-go/pkg/interfaces"
)

func (n *Node) autoconnectI2P(info *discovery.ReceivedAnnounceInfo, name string, eh []byte, peerCfg *common.InterfaceConfig) bool {
	dest := info.Info.ReachableOn
	if dest == "" {
		return false
	}
	parent, created, err := n.findOrCreateI2PParent()
	if err != nil {
		debug.Log(debug.DebugError, "Autoconnect I2P parent failed", "error", err)
		return true
	}
	peerName := name + " to " + dest
	peer := parent.AutoconnectPeer(peerName, dest, peerCfg, eh, info.RemoteIdentity)
	if !n.trackAutoconnect(peer, eh, info.Info.Type, peerName, dest, 0) {
		_ = peer.Stop()
		n.transport.UnregisterInterface(peer.GetName())
		peer.DetachAutoconnectFromParent()
		return true
	}
	if created {
		n.reloadMu.Lock()
		n.interfaces = append(n.interfaces, parent)
		n.reloadMu.Unlock()
	}
	return true
}

func (n *Node) findOrCreateI2PParent() (*interfaces.I2PInterface, bool, error) {
	n.reloadMu.Lock()
	for _, iface := range n.interfaces {
		if parent, ok := iface.(*interfaces.I2PInterface); ok {
			n.reloadMu.Unlock()
			return parent, false, nil
		}
	}
	n.reloadMu.Unlock()

	cfg := &common.InterfaceConfig{
		Type:    "I2PInterface",
		Enabled: true,
	}
	created, err := interfaces.NewFromConfigWithContext("Discovered I2P", cfg, n.fromConfigContext())
	if err != nil {
		return nil, false, err
	}
	parent, ok := created.(*interfaces.I2PInterface)
	if !ok {
		_ = created.Stop()
		return nil, false, fmt.Errorf("autoconnect: expected I2PInterface, got %T", created)
	}
	if err := parent.Start(); err != nil {
		_ = parent.Stop()
		return nil, false, err
	}
	if err := n.transport.RegisterInterface(parent.GetName(), parent); err != nil {
		_ = parent.Stop()
		return nil, false, err
	}
	n.handleInterface(parent)
	n.wireConnectivityHooks(parent)
	return parent, true, nil
}
