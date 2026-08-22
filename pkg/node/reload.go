// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"errors"
	"fmt"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/interfaces"
)

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func floatEqual(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}

func interfaceConfigsEqualForReload(a, b *common.InterfaceConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Type == b.Type &&
		a.Enabled == b.Enabled &&
		a.Address == b.Address &&
		a.TargetHost == b.TargetHost &&
		a.TargetPort == b.TargetPort &&
		a.TargetAddress == b.TargetAddress &&
		a.Port == b.Port &&
		a.KISSFraming == b.KISSFraming &&
		a.I2PTunneled == b.I2PTunneled &&
		a.I2PConnectable == b.I2PConnectable &&
		a.I2PSAMAddress == b.I2PSAMAddress &&
		a.PreferIPv6 == b.PreferIPv6 &&
		a.MaxReconnTries == b.MaxReconnTries &&
		a.Bitrate == b.Bitrate &&
		a.MTU == b.MTU &&
		sliceEqual(a.I2PPeers, b.I2PPeers) &&
		sliceEqual(a.Devices, b.Devices) &&
		sliceEqual(a.IgnoredDevices, b.IgnoredDevices) &&
		a.GroupID == b.GroupID &&
		a.DiscoveryScope == b.DiscoveryScope &&
		a.DiscoveryPort == b.DiscoveryPort &&
		a.DataPort == b.DataPort &&
		a.MulticastAddrType == b.MulticastAddrType &&
		a.Interface == b.Interface &&
		floatEqual(a.AnnounceCap, b.AnnounceCap) &&
		floatEqual(a.AnnounceRateTarget, b.AnnounceRateTarget) &&
		a.AnnounceRateGrace == b.AnnounceRateGrace &&
		floatEqual(a.AnnounceRatePenalty, b.AnnounceRatePenalty) &&
		a.IngressControl == b.IngressControl &&
		a.IngressControlSet == b.IngressControlSet &&
		a.ICNewTime == b.ICNewTime &&
		floatEqual(a.ICBurstFreqNew, b.ICBurstFreqNew) &&
		floatEqual(a.ICBurstFreq, b.ICBurstFreq) &&
		a.ICMaxHeldAnnounces == b.ICMaxHeldAnnounces &&
		a.ICBurstHold == b.ICBurstHold &&
		a.ICBurstPenalty == b.ICBurstPenalty &&
		a.ICHeldReleaseInterval == b.ICHeldReleaseInterval &&
		floatEqual(a.ICPRBurstFreqNew, b.ICPRBurstFreqNew) &&
		floatEqual(a.ICPRBurstFreq, b.ICPRBurstFreq) &&
		floatEqual(a.ECPRFreq, b.ECPRFreq) &&
		a.EgressControl == b.EgressControl &&
		a.EgressControlSet == b.EgressControlSet &&
		a.NetworkName == b.NetworkName &&
		a.Passphrase == b.Passphrase &&
		a.IFACSize == b.IFACSize &&
		a.IFACNetname == b.IFACNetname &&
		a.IFACNetkey == b.IFACNetkey &&
		a.PublishIFAC == b.PublishIFAC &&
		a.Command == b.Command &&
		a.RespawnDelay == b.RespawnDelay &&
		a.SharedInstanceType == b.SharedInstanceType &&
		a.InstanceName == b.InstanceName &&
		a.CertFile == b.CertFile &&
		a.KeyFile == b.KeyFile &&
		a.PeerKey == b.PeerKey &&
		a.SNI == b.SNI &&
		a.Mode == b.Mode &&
		a.RecursivePRs == b.RecursivePRs &&
		a.AnnouncesFromInternal == b.AnnouncesFromInternal &&
		a.AnnouncesFromInternalSet == b.AnnouncesFromInternalSet &&
		a.AnnouncesToInternal == b.AnnouncesToInternal &&
		a.AnnouncesToInternalSet == b.AnnouncesToInternalSet &&
		a.Gravity == b.Gravity &&
		a.GravitySet == b.GravitySet &&
		a.Outgoing == b.Outgoing &&
		a.OutgoingSet == b.OutgoingSet &&
		a.Device == b.Device &&
		a.Speed == b.Speed &&
		a.DataBits == b.DataBits &&
		a.Parity == b.Parity &&
		a.StopBits == b.StopBits &&
		a.RTSCTS == b.RTSCTS &&
		a.DSRDTR == b.DSRDTR &&
		a.XONXOFF == b.XONXOFF &&
		a.SerialFrameIdleMs == b.SerialFrameIdleMs &&
		a.Path == b.Path &&
		a.TransportMode == b.TransportMode &&
		a.Domain == b.Domain &&
		a.ResolveIntervalSec == b.ResolveIntervalSec &&
		a.ContextID == b.ContextID &&
		a.LongPollSec == b.LongPollSec &&
		a.Discoverable == b.Discoverable &&
		a.DiscoveryName == b.DiscoveryName &&
		a.ReachableOn == b.ReachableOn &&
		a.DiscoveryAnnounceIntervalSec == b.DiscoveryAnnounceIntervalSec &&
		a.DiscoveryStampValue == b.DiscoveryStampValue &&
		a.DiscoveryEncrypt == b.DiscoveryEncrypt &&
		a.DiscoveryLocationCmd == b.DiscoveryLocationCmd &&
		floatEqual(a.DiscoveryLatitude, b.DiscoveryLatitude) &&
		floatEqual(a.DiscoveryLongitude, b.DiscoveryLongitude) &&
		floatEqual(a.DiscoveryHeight, b.DiscoveryHeight) &&
		a.HasDiscoveryGeo == b.HasDiscoveryGeo &&
		a.ControlHost == b.ControlHost &&
		a.ControlPort == b.ControlPort &&
		a.MTUOverhead == b.MTUOverhead &&
		a.AutoFragmentation == b.AutoFragmentation &&
		a.AutoFragSet == b.AutoFragSet &&
		a.ShortFrames == b.ShortFrames &&
		a.ShortMTU == b.ShortMTU &&
		a.HandshakeX2 == b.HandshakeX2 &&
		a.ProofX2 == b.ProofX2 &&
		a.AutoBitrate == b.AutoBitrate &&
		a.AutoBitrateSet == b.AutoBitrateSet &&
		a.CSMAOverhead == b.CSMAOverhead &&
		a.CSMAOverheadSet == b.CSMAOverheadSet &&
		floatEqual(a.TimeoutMargin, b.TimeoutMargin) &&
		a.FrequencyHz == b.FrequencyHz &&
		a.SampleRate == b.SampleRate &&
		a.Bandwidth == b.Bandwidth &&
		floatEqual(a.RXGain, b.RXGain) &&
		floatEqual(a.TXGain, b.TXGain) &&
		a.Modem == b.Modem &&
		a.SerialNum == b.SerialNum
}

func (n *Node) tearDownInterface(iface interfaces.Interface) {
	if iface == nil {
		return
	}
	name := iface.GetName()
	n.transport.UnregisterInterface(name)
	n.wiringMu.Lock()
	if buf, ok := n.buffers[name]; ok {
		_ = buf.Close()
		delete(n.buffers, name)
	}
	if ch, ok := n.channels[name]; ok {
		_ = ch.Close()
		delete(n.channels, name)
	}
	n.wiringMu.Unlock()
	_ = iface.Stop()
}

// ReloadInterfaces reconciles network interfaces against newCfg without restarting transport.
func (n *Node) ReloadInterfaces(newCfg *common.ReticulumConfig) error {
	if newCfg == nil {
		return errors.New("nil config")
	}
	if n.sharedInstance != nil && !n.sharedInstance.OwnsNetworkInterfaces() {
		n.config = newCfg
		n.transport.SetReticulumConfig(newCfg)
		return nil
	}
	if n.transport == nil {
		return errors.New("nil transport")
	}

	n.reloadMu.Lock()
	defer n.reloadMu.Unlock()

	oldCfg := n.config
	oldByName := make(map[string]interfaces.Interface, len(n.interfaces))
	for _, x := range n.interfaces {
		oldByName[x.GetName()] = x
	}

	for name, oldI := range oldByName {
		ic, inNew := newCfg.Interfaces[name]
		if !inNew || !ic.Enabled {
			n.tearDownInterface(oldI)
			delete(oldByName, name)
			continue
		}
		if !interfaceConfigsEqualForReload(oldCfg.Interfaces[name], ic) {
			n.tearDownInterface(oldI)
			delete(oldByName, name)
		}
	}

	var next []interfaces.Interface
	for name, ic := range newCfg.Interfaces {
		if !ic.Enabled {
			continue
		}
		if oldI, ok := oldByName[name]; ok {
			next = append(next, oldI)
			continue
		}
		niface, err := interfaces.NewFromConfigWithContext(name, ic, n.fromConfigContext())
		if err != nil {
			if newCfg.PanicOnInterfaceErr {
				return fmt.Errorf("interface %s: %w", name, err)
			}
			debug.Log(debug.DebugError, "ReloadInterfaces: skip interface", "name", name, "error", err)
			continue
		}
		if err := niface.Start(); err != nil {
			if newCfg.PanicOnInterfaceErr {
				return fmt.Errorf("start %s: %w", name, err)
			}
			debug.Log(debug.DebugError, "ReloadInterfaces: start failed", "name", name, "error", err)
			continue
		}
		ni, ok := niface.(common.NetworkInterface)
		if !ok {
			_ = niface.Stop()
			return fmt.Errorf("interface %s does not implement common.NetworkInterface", name)
		}
		if err := n.transport.ReplaceInterface(name, ni); err != nil {
			_ = niface.Stop()
			if newCfg.PanicOnInterfaceErr {
				return err
			}
			debug.Log(debug.DebugError, "ReloadInterfaces: ReplaceInterface failed", "name", name, "error", err)
			continue
		}
		n.handleInterface(ni)
		n.wireConnectivityHooks(niface)
		next = append(next, niface)
	}

	n.interfaces = next
	n.config = newCfg
	n.transport.SetReticulumConfig(newCfg)
	return nil
}
