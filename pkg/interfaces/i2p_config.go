// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/cryptography"
)

func applyI2PParentConfig(parent *I2PInterface, cfg *common.InterfaceConfig) {
	if cfg == nil {
		return
	}
	if cfg.MTU > 0 {
		parent.MTU = cfg.MTU
	}
	if cfg.Bitrate > 0 {
		parent.Bitrate = cfg.Bitrate
	}
	if cfg.IngressControlSet {
		parent.SetIngressControl(cfg.IngressControl)
	}
	icPrBurstFreqNew := icPRBurstFreqNew
	icPrBurstFreq := icPRBurstFreq
	ecPrFreq := ecPRFreq
	egressControl := false
	if cfg.ICPRBurstFreqNew > 0 {
		icPrBurstFreqNew = cfg.ICPRBurstFreqNew
	}
	if cfg.ICPRBurstFreq > 0 {
		icPrBurstFreq = cfg.ICPRBurstFreq
	}
	if cfg.ECPRFreq > 0 {
		ecPrFreq = cfg.ECPRFreq
	}
	if cfg.EgressControlSet {
		egressControl = cfg.EgressControl
	}
	parent.SetPRBurstConfig(icPrBurstFreqNew, icPrBurstFreq, ecPrFreq, egressControl)
}

func applyI2PPeerConfig(peer *I2PInterfacePeer, cfg *common.InterfaceConfig) {
	if peer == nil || cfg == nil {
		return
	}
	if cfg.MTU > 0 {
		peer.MTU = cfg.MTU
	} else if peer.parent != nil {
		peer.MTU = peer.parent.MTU
	}
	if cfg.Bitrate > 0 {
		peer.Bitrate = cfg.Bitrate
	} else if peer.parent != nil {
		peer.Bitrate = peer.parent.Bitrate
	}
	peer.kissFraming = cfg.KISSFraming
	peer.wantsTunnel = !cfg.KISSFraming
	if cfg.IngressControlSet {
		peer.SetIngressControl(cfg.IngressControl)
	}
	icPrBurstFreqNew := icPRBurstFreqNew
	icPrBurstFreq := icPRBurstFreq
	ecPrFreq := ecPRFreq
	egressControl := false
	if cfg.ICPRBurstFreqNew > 0 {
		icPrBurstFreqNew = cfg.ICPRBurstFreqNew
	}
	if cfg.ICPRBurstFreq > 0 {
		icPrBurstFreq = cfg.ICPRBurstFreq
	}
	if cfg.ECPRFreq > 0 {
		ecPrFreq = cfg.ECPRFreq
	}
	if cfg.EgressControlSet {
		egressControl = cfg.EgressControl
	}
	peer.SetPRBurstConfig(icPrBurstFreqNew, icPrBurstFreq, ecPrFreq, egressControl)
}

// InterfaceHashFromName returns the tunnel interface hash for a peer name.
func InterfaceHashFromName(name string) []byte {
	return cryptography.Hash([]byte("I2PInterfacePeer[" + name + "]"))
}

// InterfaceConfigProvider supplies the parent [[Interface]] config for spawned peers.
type InterfaceConfigProvider interface {
	InterfaceConfig() *common.InterfaceConfig
}
