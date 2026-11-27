// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
)

func TestAttachFromRNSConfigLoadsTCPClients(t *testing.T) {
	dir := t.TempDir()
	cfg := `[reticulum]
  enable_sandbox = yes
  enable_seccomp = yes

[[Test TCP]]
  type = TCPClientInterface
  enabled = yes
  target_host = 127.0.0.1
  target_port = 4242

[[Disabled TCP]]
  type = TCPClientInterface
  enabled = no
  target_host = 127.0.0.1
  target_port = 4243
`
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	debug.Init()
	tr, _, err := startTransport(TransportConfig{RNSConfigDir: dir}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	if len(tr.GetInterfaces()) < 1 {
		t.Fatalf("expected at least one interface, got %d", len(tr.GetInterfaces()))
	}
}

func TestStartTransportAppliesStampsSection(t *testing.T) {
	dir := t.TempDir()
	cfg := `[reticulum]
  enable_transport = yes
  share_instance = no

[stamps]
  destination_cost = 1.5
  destination_value = 2.5
  pr_cost = 1.25
  pr_value = 1.75

[interfaces]
  [[Test UDP]]
    type = UDPInterface
    enabled = yes
    listen_ip = 127.0.0.1
    listen_port = 0
    forward_ip = 127.0.0.1
    forward_port = 9
`
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	debug.Init()
	tr, ifaces, err := startTransport(TransportConfig{RNSConfigDir: dir}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, iface := range ifaces {
			_ = iface.Stop()
		}
		_ = tr.Close()
	})
	if got := tr.DestinationCost(); got != 1.5 {
		t.Fatalf("destination cost = %v, want 1.5", got)
	}
	if got := tr.DestinationValue(); got != 2.5 {
		t.Fatalf("destination value = %v, want 2.5", got)
	}
	if got := tr.PRCost(); got != 1.25 {
		t.Fatalf("pr cost = %v, want 1.25", got)
	}
	if got := tr.PRValue(); got != 1.75 {
		t.Fatalf("pr value = %v, want 1.75", got)
	}
}

func TestStartTransportDefaultStampsWithoutSection(t *testing.T) {
	dir := t.TempDir()
	cfg := `[reticulum]
  enable_transport = yes
  share_instance = no

[interfaces]
  [[Test UDP]]
    type = UDPInterface
    enabled = yes
    listen_ip = 127.0.0.1
    listen_port = 0
    forward_ip = 127.0.0.1
    forward_port = 9
`
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	debug.Init()
	tr, ifaces, err := startTransport(TransportConfig{RNSConfigDir: dir}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, iface := range ifaces {
			_ = iface.Stop()
		}
		_ = tr.Close()
	})
	if got := tr.DestinationValue(); got != 25.0 {
		t.Fatalf("destination value = %v, want python default 25.0", got)
	}
}
