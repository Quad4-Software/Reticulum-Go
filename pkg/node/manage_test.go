// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/discovery"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
)

// manageTestNode builds a Node backed by a real config file containing one
// disabled UDP interface named udp_test.
func manageTestNode(t *testing.T) (*Node, int) {
	t.Helper()
	dir := t.TempDir()
	port := freeUDPPort(t)
	cfgPath := filepath.Join(dir, "config")
	conf := fmt.Sprintf(`[reticulum]
enable_transport = no
share_instance = no

[interfaces]
  [[udp_test]]
    type = UDPInterface
    enabled = no
    listen_ip = 127.0.0.1
    listen_port = %d
`, port)
	if err := os.WriteFile(cfgPath, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := common.DefaultConfig()
	cfg.ConfigPath = cfgPath
	cfg.EnableTransport = false
	cfg.ShareInstance = false
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return n, port
}

func TestAttachDetachReloadUDP(t *testing.T) {
	n, _ := manageTestNode(t)

	if got := n.DetachInterface("udp_test"); got != ManageNotFound {
		t.Fatalf("detach missing interface: got %d want ManageNotFound", got)
	}
	if got := n.AttachInterface("udp_test"); got != ManageOK {
		t.Fatalf("attach: got %d want ManageOK", got)
	}
	if _, err := n.transport.GetInterface("udp_test"); err != nil {
		t.Fatal("interface not registered after attach")
	}
	// Python returns False when attaching an existing interface.
	if got := n.AttachInterface("udp_test"); got != ManageFailed {
		t.Fatalf("attach existing: got %d want ManageFailed", got)
	}
	// Attaching a name absent from the config file maps to Python None.
	if got := n.AttachInterface("does_not_exist"); got != ManageNotFound {
		t.Fatalf("attach unconfigured: got %d want ManageNotFound", got)
	}
	if got := n.ReloadInterface("udp_test"); got != ManageOK {
		t.Fatalf("reload: got %d want ManageOK", got)
	}
	if _, err := n.transport.GetInterface("udp_test"); err != nil {
		t.Fatal("interface missing after reload")
	}
	if got := n.DetachInterface("udp_test"); got != ManageOK {
		t.Fatalf("detach: got %d want ManageOK", got)
	}
	if _, err := n.transport.GetInterface("udp_test"); err == nil {
		t.Fatal("interface still registered after detach")
	}
	if got := n.ReloadInterface("udp_test"); got != ManageNotFound {
		t.Fatalf("reload missing: got %d want ManageNotFound", got)
	}
}

func TestInterfaceManagementDisabled(t *testing.T) {
	n, _ := manageTestNode(t)
	n.config.DisableInterfaceManagement = true
	if got := n.AttachInterface("udp_test"); got != ManageFailed {
		t.Fatalf("attach disabled: got %d want ManageFailed", got)
	}
	if got := n.DetachInterface("udp_test"); got != ManageFailed {
		t.Fatalf("detach disabled: got %d want ManageFailed", got)
	}
}

func TestDetachRefusesLocalInterface(t *testing.T) {
	n, _ := manageTestNode(t)
	// Register a LocalClientInterface shell: Python _detach_interface refuses
	// I2PInterface, LocalClientInterface and LocalServerInterface outright.
	cli, err := interfaces.NewLocalClientInterface(37428, "/nonexistent.sock", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.transport.RegisterInterface("local_test", cli); err != nil {
		t.Fatal(err)
	}
	defer n.transport.UnregisterInterface("local_test")
	if got := n.DetachInterface("local_test"); got != ManageFailed {
		t.Fatalf("detach local client: got %d want ManageFailed", got)
	}
}

func TestAutoconnectInterfaceNameSequential(t *testing.T) {
	n, _ := manageTestNode(t)
	u1, err := interfaces.NewUDPInterface("peer (10.0.0.1:4242)", "127.0.0.1:0", "", true)
	if err != nil {
		t.Fatal(err)
	}
	u2, err := interfaces.NewUDPInterface("peer (10.0.0.1:4242) (2)", "127.0.0.1:0", "", true)
	if err != nil {
		t.Fatal(err)
	}
	defer u1.Stop()
	defer u2.Stop()
	if err := n.transport.RegisterInterface(u1.Name, u1); err != nil {
		t.Fatal(err)
	}
	if err := n.transport.RegisterInterface(u2.Name, u2); err != nil {
		t.Fatal(err)
	}
	defer n.transport.UnregisterInterface(u1.Name)
	defer n.transport.UnregisterInterface(u2.Name)

	info := &discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{
			Type:        "BackboneInterface",
			Name:        "peer",
			ReachableOn: "10.0.0.1",
			Port:        4242,
			HasPort:     true,
		},
	}
	got := n.autoconnectInterfaceName(info)
	if got != "peer (10.0.0.1:4242) (3)" {
		t.Fatalf("sequential name got %q", got)
	}
}
