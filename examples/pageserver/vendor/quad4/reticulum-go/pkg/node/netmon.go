// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package node

import (
	"time"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/health"
)

const interfaceMonitorInterval = 10 * time.Second

// startInterfaceMonitor polls OS network interfaces and calls OnNetworkAvailable
// when link state or addresses change. Uses net.Interfaces and works on Linux,
// Android, Windows, macOS, and BSD on any CPU architecture.
func (n *Node) startInterfaceMonitor() {
	go func() {
		ticker := time.NewTicker(interfaceMonitorInterval)
		defer ticker.Stop()
		last := currentInterfaceSnapshot()
		for range ticker.C {
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
