// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package main

import (
	"errors"
	"fmt"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/interfaces"
)

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
		a.GroupID == b.GroupID &&
		a.DiscoveryScope == b.DiscoveryScope &&
		a.DiscoveryPort == b.DiscoveryPort &&
		a.DataPort == b.DataPort &&
		a.MulticastAddrType == b.MulticastAddrType &&
		a.Interface == b.Interface
}

func (r *Reticulum) tearDownInterface(iface interfaces.Interface) {
	if iface == nil {
		return
	}
	name := iface.GetName()
	r.transport.UnregisterInterface(name)
	if buf, ok := r.buffers[name]; ok {
		if err := buf.Close(); err != nil {
			debug.Log(debug.DebugVerbose, "buffer close", "name", name, "error", err)
		}
		delete(r.buffers, name)
	}
	if ch, ok := r.channels[name]; ok {
		if err := ch.Close(); err != nil {
			debug.Log(debug.DebugVerbose, "channel close", "name", name, "error", err)
		}
		delete(r.channels, name)
	}
	if err := iface.Stop(); err != nil {
		debug.Log(debug.DebugVerbose, "interface stop", "name", name, "error", err)
	}
}

// ReloadInterfaces reconciles network interfaces against newCfg without
// restarting the transport or identity. Disabled or removed interfaces are
// stopped and unregistered; unchanged enabled entries are kept; new or
// reconfigured entries are built, started, and registered via ReplaceInterface.
func (r *Reticulum) ReloadInterfaces(newCfg *common.ReticulumConfig) error {
	if newCfg == nil {
		return errors.New("nil config")
	}
	if r.transport == nil {
		return errors.New("nil transport")
	}

	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	oldCfg := r.config
	oldByName := make(map[string]interfaces.Interface, len(r.interfaces))
	for _, x := range r.interfaces {
		oldByName[x.GetName()] = x
	}

	for name, oldI := range oldByName {
		ic, inNew := newCfg.Interfaces[name]
		if !inNew || !ic.Enabled {
			r.tearDownInterface(oldI)
			delete(oldByName, name)
			continue
		}
		if !interfaceConfigsEqualForReload(oldCfg.Interfaces[name], ic) {
			r.tearDownInterface(oldI)
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
		niface, err := interfaces.NewFromConfig(name, ic)
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
		if err := r.transport.ReplaceInterface(name, ni); err != nil {
			_ = niface.Stop()
			if newCfg.PanicOnInterfaceErr {
				return err
			}
			debug.Log(debug.DebugCritical, "ReloadInterfaces: ReplaceInterface failed", "name", name, "error", err)
			continue
		}
		r.handleInterface(ni)
		next = append(next, niface)
	}

	r.interfaces = next
	r.config = newCfg
	r.transport.SetReticulumConfig(newCfg)
	return nil
}
