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
