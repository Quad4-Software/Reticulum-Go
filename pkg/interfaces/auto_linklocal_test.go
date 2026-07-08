// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"net"
	"os/exec"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func setupRegressionVeth(t *testing.T, base string) (string, func()) {
	t.Helper()
	name := base
	peer := base + "p"
	_ = exec.Command("ip", "link", "del", name).Run()
	out, err := exec.Command("ip", "link", "add", name, "type", "veth", "peer", "name", peer).CombinedOutput()
	if err != nil {
		t.Skipf("veth not available: %v\n%s", err, out)
	}
	exec.Command("ip", "link", "set", name, "up").Run()
	exec.Command("ip", "link", "set", peer, "up").Run()
	exec.Command("ip", "-6", "addr", "add", "fe80::1/64", "dev", name, "nodad").Run()
	time.Sleep(100 * time.Millisecond)
	return name, func() { _ = exec.Command("ip", "link", "del", name).Run() }
}

func TestSelectLinkLocalAddrPrefersBindable(t *testing.T) {
	ifaceName, cleanup := setupRegressionVeth(t, "rnsll0")
	defer cleanup()

	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		t.Fatalf("InterfaceByName: %v", err)
	}

	selected := selectLinkLocalAddr(iface, DefaultDiscoveryPort+1)
	if selected == "" {
		t.Fatal("expected a link-local address")
	}
	if !canBindLinkLocalUDP(iface, selected, DefaultDiscoveryPort+1) {
		t.Fatalf("selected address %q is not bindable", selected)
	}
	if selected != "fe80::1" {
		t.Fatalf("selected = %q; want fe80::1 when manual nodad address is bindable", selected)
	}
}

func TestAutoInterfaceConfiguresBindableLinkLocalOnVeth(t *testing.T) {
	ifaceName, cleanup := setupRegressionVeth(t, "rnsll1")
	defer cleanup()

	ai, err := NewAutoInterface("auto", &common.InterfaceConfig{
		Enabled: true,
		Devices: []string{ifaceName},
	})
	if err != nil {
		t.Fatalf("NewAutoInterface: %v", err)
	}
	if err := ai.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ai.Stop()

	ai.Mutex.RLock()
	adopted, ok := ai.adoptedInterfaces[ifaceName]
	hasOutbound := ai.outboundConns[ifaceName] != nil
	ai.Mutex.RUnlock()

	if !ok {
		t.Fatal("interface not adopted")
	}
	if adopted.linkLocalAddr != "fe80::1" {
		t.Fatalf("linkLocalAddr = %q; want fe80::1", adopted.linkLocalAddr)
	}
	if !hasOutbound {
		t.Fatal("expected outbound socket for adopted interface")
	}
}
