// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package node

import (
	"net"
	"testing"
)

func TestInterfaceSnapshotsEqual(t *testing.T) {
	a := []ifaceSnapshot{{name: "eth0", up: true, addrs: "10.0.0.1/24"}}
	b := []ifaceSnapshot{{name: "eth0", up: true, addrs: "10.0.0.1/24"}}
	if !interfaceSnapshotsEqual(a, b) {
		t.Fatal("expected equal snapshots")
	}
	b[0].up = false
	if interfaceSnapshotsEqual(a, b) {
		t.Fatal("expected unequal when up flag differs")
	}
}

func TestMonitorInterfaceSkipsLoopback(t *testing.T) {
	if monitorInterface(&net.Interface{Name: "lo", Flags: net.FlagUp | net.FlagLoopback}) {
		t.Fatal("loopback should be ignored")
	}
	if !monitorInterface(&net.Interface{Name: "eth0", Flags: net.FlagUp}) {
		t.Fatal("non-loopback should be monitored")
	}
}

func TestCurrentInterfaceSnapshotSorted(t *testing.T) {
	snap := currentInterfaceSnapshot()
	for i := 1; i < len(snap); i++ {
		if snap[i-1].name > snap[i].name {
			t.Fatalf("snapshots not sorted: %s before %s", snap[i-1].name, snap[i].name)
		}
	}
}
