// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package node

import (
	"net"
	"sort"
)

type ifaceSnapshot struct {
	name string
	up   bool
}

func currentInterfaceSnapshot() []ifaceSnapshot {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	out := make([]ifaceSnapshot, 0, len(ifaces))
	for _, iface := range ifaces {
		out = append(out, ifaceSnapshot{
			name: iface.Name,
			up:   iface.Flags&net.FlagUp != 0,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func interfaceSnapshotsEqual(a, b []ifaceSnapshot) bool {
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
