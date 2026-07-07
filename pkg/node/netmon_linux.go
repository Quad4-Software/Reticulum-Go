// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package node

import (
	"time"

	"quad4/reticulum-go/pkg/debug"
)

func (n *Node) startInterfaceMonitor() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		last := currentInterfaceSnapshot()
		for range ticker.C {
			cur := currentInterfaceSnapshot()
			if !interfaceSnapshotsEqual(last, cur) {
				last = cur
				if err := n.OnNetworkAvailable(); err != nil {
					debug.Log(debug.DebugVerbose, "interface monitor refresh", "error", err)
				}
			}
		}
	}()
}
