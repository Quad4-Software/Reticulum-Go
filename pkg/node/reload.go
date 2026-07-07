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
		a.Port == b.Port &&
		a.KISSFraming == b.KISSFraming &&
		a.I2PTunneled == b.I2PTunneled &&
		a.I2PConnectable == b.I2PConnectable &&
		a.I2PSAMAddress == b.I2PSAMAddress &&
		a.MaxReconnTries == b.MaxReconnTries &&
		sliceEqual(a.I2PPeers, b.I2PPeers) &&
		sliceEqual(a.Devices, b.Devices) &&
		sliceEqual(a.IgnoredDevices, b.IgnoredDevices) &&
		a.GroupID == b.GroupID &&
		a.DiscoveryScope == b.DiscoveryScope &&
		a.DiscoveryPort == b.DiscoveryPort &&
		a.DataPort == b.DataPort &&
		a.MulticastAddrType == b.MulticastAddrType &&
		a.Interface == b.Interface &&
		a.NetworkName == b.NetworkName &&
		a.Passphrase == b.Passphrase &&
		a.IFACSize == b.IFACSize &&
		a.IFACNetname == b.IFACNetname &&
		a.IFACNetkey == b.IFACNetkey &&
		a.Command == b.Command &&
		a.RespawnDelay == b.RespawnDelay &&
		a.SharedInstanceType == b.SharedInstanceType &&
		a.InstanceName == b.InstanceName
}

func (n *Node) tearDownInterface(iface interfaces.Interface) {
	if iface == nil {
		return
	}
	name := iface.GetName()
	n.transport.UnregisterInterface(name)
	if buf, ok := n.buffers[name]; ok {
		_ = buf.Close()
		delete(n.buffers, name)
	}
	if ch, ok := n.channels[name]; ok {
		_ = ch.Close()
		delete(n.channels, name)
	}
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
			debug.Log(debug.DebugCritical, "ReloadInterfaces: skip interface", "name", name, "error", err)
			continue
		}
		if err := niface.Start(); err != nil {
			if newCfg.PanicOnInterfaceErr {
				return fmt.Errorf("start %s: %w", name, err)
			}
			debug.Log(debug.DebugCritical, "ReloadInterfaces: start failed", "name", name, "error", err)
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
			debug.Log(debug.DebugCritical, "ReloadInterfaces: ReplaceInterface failed", "name", name, "error", err)
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
