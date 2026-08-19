// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"errors"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/transport"
)

// OnNetworkAvailable resumes interfaces and refreshes paths after sleep or wake.
func (n *Node) OnNetworkAvailable() error {
	n.reloadMu.Lock()
	defer n.reloadMu.Unlock()
	if n.transport == nil {
		return errors.New("nil transport")
	}
	downDuration := time.Duration(0)
	if n.networkPaused && !n.lastNetworkDown.IsZero() {
		downDuration = time.Since(n.lastNetworkDown)
	}
	n.networkPaused = false
	link.SetGlobalPaused(false)

	if !n.ownsInterfaces() {
		return n.RefreshPathsWithDownDuration(downDuration)
	}

	for _, iface := range n.interfaces {
		if rescanner, ok := iface.(*interfaces.AutoInterface); ok {
			_ = rescanner.RescanInterfaces()
		}
		if !iface.IsOnline() {
			if err := iface.Start(); err != nil {
				debug.Log(debug.DebugError, "OnNetworkAvailable: start failed", "name", iface.GetName(), "error", err)
			} else if ni, ok := iface.(common.NetworkInterface); ok {
				if err := n.transport.RegisterInterface(iface.GetName(), ni); err != nil {
					_ = n.transport.ReplaceInterface(iface.GetName(), ni)
				}
				if _, ok := n.buffers[iface.GetName()]; !ok {
					n.handleInterface(ni)
				}
			}
		} else {
			iface.Enable()
		}
	}
	if n.linkMgr != nil {
		n.linkMgr.onNetworkAvailable()
	}
	return n.RefreshPathsWithDownDuration(downDuration)
}

// OnNetworkLost pauses interfaces when the host reports network loss or sleep.
func (n *Node) OnNetworkLost() error {
	n.reloadMu.Lock()
	defer n.reloadMu.Unlock()
	n.lastNetworkDown = time.Now()
	n.networkPaused = true
	link.SetGlobalPaused(true)

	if !n.ownsInterfaces() {
		return nil
	}
	for _, iface := range n.interfaces {
		switch n.pauseMode {
		case PauseModeStop:
			_ = iface.Stop()
		default:
			iface.Disable()
		}
	}
	return nil
}

// SetPauseMode configures how OnNetworkLost affects interfaces.
func (n *Node) SetPauseMode(mode PauseMode) {
	n.pauseMode = mode
}

// RefreshPaths refreshes paths for watched destinations and explicit arguments.
func (n *Node) RefreshPaths(dests ...[]byte) error {
	return n.RefreshPathsWithDownDuration(0, dests...)
}

// RefreshPathsWithDownDuration refreshes paths, expiring stale routes after long outages.
func (n *Node) RefreshPathsWithDownDuration(downDuration time.Duration, dests ...[]byte) error {
	if n.transport == nil {
		return errors.New("nil transport")
	}
	ttl := time.Duration(transport.PathRequestTTL) * time.Second
	targets := n.collectRefreshTargets(dests...)
	for _, hash := range targets {
		if downDuration > ttl || !n.transport.HasPath(hash) {
			n.transport.ExpirePath(hash)
		}
		n.transport.PrepareFreshPathRequest(hash)
	}
	return nil
}

func (n *Node) collectRefreshTargets(extra ...[]byte) [][]byte {
	seen := make(map[string]struct{})
	var out [][]byte
	add := func(hash []byte) {
		if len(hash) != 16 {
			return
		}
		key := fmt.Sprintf("%x", hash)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, append([]byte(nil), hash...))
	}
	n.watchMu.RLock()
	for _, hash := range n.watchedDests {
		add(hash)
	}
	n.watchMu.RUnlock()
	for _, hash := range extra {
		add(hash)
	}
	return out
}
