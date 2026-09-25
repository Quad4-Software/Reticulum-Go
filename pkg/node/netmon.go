// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package node

import (
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/health"
)

const interfaceMonitorInterval = 10 * time.Second

// startInterfaceMonitor polls OS network interfaces and calls OnNetworkAvailable
// when link state or addresses change. Uses net.Interfaces and works on Linux,
// Android, Windows, macOS, and BSD on any CPU architecture.
func (n *Node) startInterfaceMonitor() {
	n.netmonMu.Lock()
	stop := make(chan struct{})
	n.netmonStop = stop
	n.netmonMu.Unlock()
	go func() {
		ticker := time.NewTicker(interfaceMonitorInterval)
		defer ticker.Stop()
		last := currentInterfaceSnapshot()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			cur := currentInterfaceSnapshot()
			if interfaceSnapshotsEqual(last, cur) {
				continue
			}
			last = cur
			health.Inc("", health.KindNetmonFlap)
			if err := n.OnNetworkAvailable(); err != nil {
				debug.Log(debug.DebugVerbose, "interface monitor refresh", "error", err)
			}
		}
	}()
}
