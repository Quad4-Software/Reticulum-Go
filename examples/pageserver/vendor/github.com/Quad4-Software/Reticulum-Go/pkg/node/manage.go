// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	"github.com/Quad4-Software/Reticulum-Go/pkg/reticulumconfig"
	"github.com/Quad4-Software/Reticulum-Go/pkg/rnsutil"
)

// ManageOutcome mirrors the Python True/False/None tri-state returned by
// Reticulum._attach_interface, _detach_interface and _reload_interface.
type ManageOutcome int

const (
	// ManageFailed maps to Python False: refused, disabled, or error.
	ManageFailed ManageOutcome = iota
	// ManageOK maps to Python True.
	ManageOK
	// ManageNotFound maps to Python None: no such running interface, or for
	// attach no matching configuration entry exists.
	ManageNotFound
)

// RPCValue renders the outcome for the shared instance RPC wire format:
// true, false, or nil exactly like Python rpc_return.
func (o ManageOutcome) RPCValue() any {
	switch o {
	case ManageOK:
		return true
	case ManageNotFound:
		return nil
	default:
		return false
	}
}

// spawnedLister is implemented by interfaces that own child interfaces which
// are registered in transport under their own names.
type spawnedLister interface {
	SpawnedInterfaces() []interfaces.Interface
}

// spawnedInterfaces enumerates children of a parent interface for detach,
// matching Python _detach_interface's spawned_interfaces walk.
func spawnedInterfaces(iface common.NetworkInterface) []interfaces.Interface {
	switch p := iface.(type) {
	case *interfaces.BackboneInterface:
		return p.SpawnedInterfaces()
	case *interfaces.RNodeMultiInterface:
		subs := p.SubInterfaces()
		out := make([]interfaces.Interface, 0, len(subs))
		for _, s := range subs {
			out = append(out, s)
		}
		return out
	case *interfaces.AwareInterface:
		return p.SpawnedInterfaces()
	case *interfaces.I2PInterface:
		return p.ListSpawnedPeers()
	case spawnedLister:
		return p.SpawnedInterfaces()
	}
	return nil
}

// undetachable reports whether Python _detach_interface refuses this class:
// I2PInterface and both Local interface roles are not detachable.
func undetachable(iface common.NetworkInterface) bool {
	switch iface.(type) {
	case *interfaces.I2PInterface, *interfaces.LocalClientInterface, *interfaces.LocalServerInterface:
		return true
	}
	return false
}

// dropInterface stops and unregisters iface plus any state the node tracks
// for it. Used by detach and the autoconnect teardown path.
func (n *Node) dropInterface(iface interfaces.Interface) {
	if iface == nil {
		return
	}
	name := iface.GetName()
	iface.Detach()
	_ = iface.Stop()
	n.transport.UnregisterInterface(name)
	n.unregisterInterfaceBuffers(name)

	n.reloadMu.Lock()
	kept := n.interfaces[:0]
	for _, cur := range n.interfaces {
		if cur != iface {
			kept = append(kept, cur)
		}
	}
	n.interfaces = kept
	n.reloadMu.Unlock()

	n.acMu.Lock()
	keptAC := n.acEntries[:0]
	for _, e := range n.acEntries {
		if e.iface != iface {
			keptAC = append(keptAC, e)
		}
	}
	n.acEntries = keptAC
	n.acMu.Unlock()
}

// manageViaSharedRPC forwards a manage action to the owning shared instance
// when this node is attached as a shared-instance client, matching Python's
// public attach/detach/reload_interface wrappers.
func (n *Node) manageViaSharedRPC(action, name string) (any, bool) {
	if n == nil || n.sharedInstance == nil || n.sharedInstance.OwnsNetworkInterfaces() {
		return nil, false
	}
	client, err := rnsutil.DialRPC(n.config, nil)
	if err != nil {
		debug.Log(debug.DebugError, "Interface manage RPC dial failed", "error", err)
		return false, true
	}
	out, err := client.ManageInterface(action, name)
	if err != nil {
		debug.Log(debug.DebugError, "Interface manage RPC failed", "error", err)
		return false, true
	}
	return out, true
}

// rpcOutcome converts a shared-instance RPC tri-state reply to a
// ManageOutcome.
func rpcOutcome(v any) ManageOutcome {
	if v == nil {
		return ManageNotFound
	}
	if b, ok := v.(bool); ok && b {
		return ManageOK
	}
	return ManageFailed
}

// DetachInterface detaches a running interface by name, matching Python
// Reticulum._detach_interface (RNS 1.5.5). I2P and local shared-instance
// interfaces cannot be detached. Spawned children are detached first.
func (n *Node) DetachInterface(name string) ManageOutcome {
	if out, ok := n.manageViaSharedRPC("detach_interface", name); ok {
		return rpcOutcome(out)
	}
	if n == nil || n.config == nil || n.transport == nil {
		return ManageFailed
	}
	if n.config.DisableInterfaceManagement {
		return ManageFailed
	}
	iface, err := n.transport.GetInterface(name)
	if err != nil || iface == nil {
		debug.Log(debug.DebugWarning, "Attempt to detach non-existing interface", "name", name)
		return ManageNotFound
	}
	if undetachable(iface) {
		return ManageFailed
	}

	for _, child := range spawnedInterfaces(iface) {
		if child == nil {
			continue
		}
		n.dropInterface(child)
	}

	ni, ok := iface.(interfaces.Interface)
	if !ok {
		return ManageFailed
	}
	n.dropInterface(ni)
	debug.Log(debug.DebugInfo, "Interface detached", "name", name)
	return ManageOK
}

// AttachInterface synthesizes and starts an interface from the on-disk config
// by name, matching Python Reticulum._attach_interface (RNS 1.5.5). The
// enabled flag is bypassed (Python force_attach): an interface listed in the
// config file attaches even when disabled there.
func (n *Node) AttachInterface(name string) ManageOutcome {
	if out, ok := n.manageViaSharedRPC("attach_interface", name); ok {
		return rpcOutcome(out)
	}
	if n == nil || n.config == nil || n.transport == nil {
		return ManageFailed
	}
	if n.config.DisableInterfaceManagement {
		return ManageFailed
	}
	if existing, err := n.transport.GetInterface(name); err == nil && existing != nil {
		debug.Log(debug.DebugWarning, "Attempt to attach existing interface", "name", name)
		return ManageFailed
	}
	cfgPath := n.config.ConfigPath
	if cfgPath == "" {
		return ManageNotFound
	}
	fresh, err := reticulumconfig.LoadConfig(cfgPath)
	if err != nil {
		debug.Log(debug.DebugError, "Could not parse the configuration during interface attach", "path", cfgPath, "error", err)
		return ManageNotFound
	}
	ic, ok := fresh.Interfaces[name]
	if !ok || ic == nil {
		debug.Log(debug.DebugError, "Cannot attach interface, no configuration entry exists", "name", name)
		return ManageNotFound
	}

	ic.Enabled = true
	iface, err := interfaces.NewFromConfigWithContext(name, ic, n.fromConfigContext())
	if err != nil {
		debug.Log(debug.DebugError, "Interface attach failed at creation", "name", name, "error", err)
		return ManageFailed
	}
	if err := iface.Start(); err != nil {
		debug.Log(debug.DebugError, "Interface attach failed at start", "name", name, "error", err)
		_ = iface.Stop()
		return ManageFailed
	}
	ni, ok := iface.(common.NetworkInterface)
	if !ok {
		_ = iface.Stop()
		return ManageFailed
	}
	if err := n.transport.RegisterInterface(name, ni); err != nil {
		_ = iface.Stop()
		return ManageFailed
	}
	n.handleInterface(ni)
	n.wireConnectivityHooks(iface)

	n.reloadMu.Lock()
	n.interfaces = append(n.interfaces, iface)
	n.reloadMu.Unlock()

	debug.Log(debug.DebugInfo, "Interface attached", "name", name)
	return ManageOK
}

// ReloadInterface detaches then attaches an interface by name, matching
// Python Reticulum._reload_interface (RNS 1.5.5).
func (n *Node) ReloadInterface(name string) ManageOutcome {
	if out, ok := n.manageViaSharedRPC("reload_interface", name); ok {
		return rpcOutcome(out)
	}
	if n == nil || n.config == nil || n.transport == nil {
		return ManageFailed
	}
	if n.config.DisableInterfaceManagement {
		return ManageFailed
	}
	iface, err := n.transport.GetInterface(name)
	if err != nil || iface == nil {
		debug.Log(debug.DebugWarning, "Attempt to reload non-existing interface", "name", name)
		return ManageNotFound
	}
	if n.DetachInterface(name) == ManageOK && n.AttachInterface(name) == ManageOK {
		debug.Log(debug.DebugInfo, "Interface reloaded", "name", name)
		return ManageOK
	}
	debug.Log(debug.DebugInfo, "Could not reload interface", "name", name)
	return ManageFailed
}

// manageInterfaceRPC serves the shared instance "manage" RPC path used by
// rnstatus --attach/--detach/--reload (RNS 1.5.5).
func (n *Node) manageInterfaceRPC(action, name string) any {
	switch action {
	case "attach_interface":
		return n.AttachInterface(name).RPCValue()
	case "detach_interface":
		return n.DetachInterface(name).RPCValue()
	case "reload_interface":
		return n.ReloadInterface(name).RPCValue()
	}
	return nil
}
