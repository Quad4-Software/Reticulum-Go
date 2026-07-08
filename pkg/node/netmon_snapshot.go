// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js && !tinygo

package node

import (
	"net"
	"sort"
	"strings"
)

type ifaceSnapshot struct {
	name  string
	up    bool
	addrs string
}

func currentInterfaceSnapshot() []ifaceSnapshot {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	out := make([]ifaceSnapshot, 0, len(ifaces))
	for i := range ifaces {
		iface := &ifaces[i]
		if !monitorInterface(iface) {
			continue
		}
		snap := ifaceSnapshot{
			name: iface.Name,
			up:   iface.Flags&net.FlagUp != 0,
		}
		if snap.up {
			snap.addrs = interfaceAddrsKey(iface)
		}
		out = append(out, snap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func monitorInterface(iface *net.Interface) bool {
	if iface == nil {
		return false
	}
	if iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	return true
}

func interfaceAddrsKey(iface *net.Interface) string {
	addrs, err := iface.Addrs()
	if err != nil || len(addrs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		parts = append(parts, addr.String())
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
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
