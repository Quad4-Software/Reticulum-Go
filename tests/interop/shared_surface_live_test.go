// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live full-surface shared-instance RPC interop, both directions.
// Set RUN_LIVE_INTEROP=1.

package interop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/node"
	"github.com/Quad4-Software/Reticulum-Go/pkg/rnsutil"
)

// writeSharedInstanceConfig emits a minimal TCP shared-instance config.
// rpcKeyHex may be empty to let the implementation derive the authkey.
func writeSharedInstanceConfig(t *testing.T, dir string, port, ctrlPort int, rpcKeyHex string) string {
	t.Helper()
	lines := []string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
	}
	if rpcKeyHex != "" {
		lines = append(lines, "rpc_key = "+rpcKeyHex)
	}
	lines = append(lines, "")
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// startGoSharedInstance boots a Go node serving a shared instance over TCP.
func startGoSharedInstance(t *testing.T, port, ctrlPort int, cfgPath string) *node.Node {
	t.Helper()
	cfg := &common.ReticulumConfig{
		ShareInstance:       true,
		SharedInstancePort:  port,
		InstanceControlPort: ctrlPort,
		SharedInstanceType:  common.SharedInstanceTCP,
		EnableTransport:     false,
		ConfigPath:          cfgPath,
	}
	n, err := node.New(cfg)
	if err != nil {
		t.Fatalf("node new: %v", err)
	}
	if err := n.Start(); err != nil {
		t.Fatalf("node start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return n
}

// TestLiveInteropPythonSurfaceAgainstGoSharedInstance runs the exhaustive
// Python probe plus the real rnstatus CLI against a Go shared instance.
// Nil answers on keys Python tools compare numerically crash clients, so
// the probe asserts strict numeric replies where the schema requires them.
func TestLiveInteropPythonSurfaceAgainstGoSharedInstance(t *testing.T) {
	liveOrSkip(t)

	port := freeUDPPort(t)
	ctrlPort := port + 1
	cfgDir := t.TempDir()
	cfgPath := writeSharedInstanceConfig(t, cfgDir, port, ctrlPort, "")
	startGoSharedInstance(t, port, ctrlPort, cfgPath)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "rpc_surface_probe.py"))
	cmd.Env = append(os.Environ(), "INTEROP_CONFIG_DIR="+cfgDir)
	out, err := cmd.CombinedOutput()
	t.Logf("probe output:\n%s", out)
	if err != nil {
		t.Fatalf("python rpc surface probe: %v", err)
	}
	if !strings.Contains(string(out), "OK") {
		t.Fatalf("python rpc surface probe failed: %s", out)
	}

	rnstatus, err := exec.LookPath("rnstatus")
	if err != nil {
		t.Log("rnstatus not on PATH, skipping CLI check")
		return
	}
	cmd = exec.CommandContext(ctx, rnstatus, "--config", cfgDir, "-a")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rnstatus against Go shared instance: %v\n%s", err, out)
	}
	cmd = exec.CommandContext(ctx, rnstatus, "--config", cfgDir, "-j")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rnstatus --json: %v\n%s", err, out)
	}
	var decoded struct {
		Interfaces []map[string]any `json:"interfaces"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("rnstatus --json output did not parse: %v\n%s", err, out)
	}
}

// TestLiveInteropGoSurfaceAgainstPythonSharedInstance boots a real Python
// rnsd shared instance and exercises every Go RPC getter plus the rgostatus
// CLI against it.
func TestLiveInteropGoSurfaceAgainstPythonSharedInstance(t *testing.T) {
	liveOrSkip(t)
	rnsd, err := exec.LookPath("rnsd")
	if err != nil {
		t.Skip("rnsd not on PATH")
	}
	if out, err := exec.Command(pythonExe(), "-c", "import RNS").CombinedOutput(); err != nil {
		t.Skipf("python RNS unavailable: %v\n%s", err, out)
	}

	port := freeUDPPort(t)
	ctrlPort := port + 1
	cfgDir := t.TempDir()
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("rand: %v", err)
	}
	keyHex := hex.EncodeToString(keyBytes)
	writeSharedInstanceConfig(t, cfgDir, port, ctrlPort, keyHex)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, rnsd, "--config", cfgDir, "-q")
	cmd.Stdout = nil
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("rnsd start: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	rpcCfg := &common.ReticulumConfig{
		SharedInstanceType:  common.SharedInstanceTCP,
		InstanceControlPort: ctrlPort,
		RPCKey:              keyBytes,
		ConfigPath:          filepath.Join(cfgDir, "config"),
	}
	// DialRPC only builds the client; probe with a real call until rnsd's
	// control listener is accepting connections.
	client, derr := rnsutil.DialRPC(rpcCfg, keyBytes)
	if derr != nil {
		t.Fatalf("DialRPC to python rnsd: %v\n%s", derr, stderr.String())
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err = client.GetLinkCount(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("python rnsd rpc never answered: %v\n%s", err, stderr.String())
		}
		time.Sleep(250 * time.Millisecond)
	}

	dest := make([]byte, 16)
	calls := map[string]func() error{
		"link_count":        func() error { _, e := client.GetLinkCount(); return e },
		"active_links":      func() error { _, e := client.GetActiveLinkCount(); return e },
		"path_table":        func() error { _, e := client.GetPathTable(nil); return e },
		"rate_table":        func() error { _, e := client.GetRateTable(); return e },
		"interface_stats":   func() error { _, e := client.GetInterfaceStats(); return e },
		"next_hop":          func() error { _, e := client.GetNextHop(dest); return e },
		"next_hop_if_name":  func() error { _, e := client.GetNextHopIfName(dest); return e },
		"first_hop_timeout": func() error { _, e := client.GetFirstHopTimeout(dest); return e },
		"lowest_bitrate":    func() error { _, e := client.GetLowestInterfaceBitrate(); return e },
		"medium_path_timeout": func() error {
			v, e := client.GetMediumPathTimeout()
			if e == nil && v < 0 {
				return fmt.Errorf("medium_path_timeout = %v, want >= 0", v)
			}
			return e
		},
		"blackholed": func() error { _, e := client.GetBlackholedIdentities(); return e },
		"profiling":  func() error { _, e := client.GetProfilingResults(); return e },
	}
	for name, fn := range calls {
		if err := fn(); err != nil {
			t.Fatalf("%s against python rnsd: %v", name, err)
		}
	}

	// The Go CLI wrapper must run clean against a Python shared instance too.
	bin := filepath.Join(repoRoot(t), "bin", "rgostatus")
	build := exec.Command("go", "build", "-o", bin, "./cmd/rgostatus") // #nosec G204 -- test build
	build.Dir = repoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build rgostatus: %v\n%s", err, out)
	}
	cmd = exec.CommandContext(ctx, bin, "-config", cfgDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rgostatus against python rnsd: %v\n%s", err, out)
	}
}
