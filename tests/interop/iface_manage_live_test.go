// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live interface-management interop for the RNS 1.5.5 attach/detach/reload
// RPC path. Set RUN_LIVE_INTEROP=1.

package interop

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/cli"
	"github.com/Quad4-Software/Reticulum-Go/pkg/node"
	"github.com/Quad4-Software/Reticulum-Go/pkg/rnsutil"
)

// TestLiveManagePythonClientAgainstGoSharedInstance runs the full
// attach/detach/reload tri-state matrix from a Python 1.5.5 shared-instance
// client against a Go instance that owns the interfaces.
func TestLiveManagePythonClientAgainstGoSharedInstance(t *testing.T) {
	liveOrSkip(t)

	port := freeUDPPort(t)
	ctrlPort := port + 1
	ifacePort := freeUDPPort(t)
	cfgDir := t.TempDir()
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
		"[interfaces]",
		"  [[mgmt_udp]]",
		"    type = UDPInterface",
		"    enabled = yes",
		"    listen_ip = 127.0.0.1",
		"    listen_port = " + strconv.Itoa(ifacePort),
		"",
	}, "\n")
	cfgPath := filepath.Join(cfgDir, "config")
	if err := os.WriteFile(cfgPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	goCfg, err := rnsutil.LoadConfigDir(cfgDir)
	if err != nil {
		t.Fatalf("load config dir: %v", err)
	}
	n, err := node.New(goCfg)
	if err != nil {
		t.Fatalf("node new: %v", err)
	}
	if err := n.Start(); err != nil {
		t.Fatalf("node start: %v", err)
	}
	defer n.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "manage_probe.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_CONFIG_DIR="+cfgDir,
		"INTEROP_IFACE=mgmt_udp",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python manage probe: %v\n%s", err, out)
	}
	want := map[string]string{
		"detach_running":  "TRUE",
		"detach_missing":  "NONE",
		"attach_config":   "TRUE",
		"attach_existing": "FALSE",
		"reload_running":  "TRUE",
		"attach_noconfig": "NONE",
		"detach_noconfig": "NONE",
		"reload_noconfig": "NONE",
	}
	for line := range strings.Lines(strings.TrimSpace(string(out))) {
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			// Python log lines (identity created etc.) also reach stdout.
			continue
		}
		w, present := want[k]
		if !present {
			t.Fatalf("unexpected probe key %q", k)
		}
		if v != w {
			t.Fatalf("%s got %s want %s (full output: %s)", k, v, w, out)
		}
		delete(want, k)
	}
	if len(want) > 0 {
		t.Fatalf("missing probe results: %v (output: %s)", want, out)
	}

	// Final state: mgmt_udp must be attached again after reload.
	if _, err := n.Transport().GetInterface("mgmt_udp"); err != nil {
		t.Fatalf("mgmt_udp missing after reload: %v", err)
	}
}

// TestLiveManageRgostatusAgainstPythonRNSD drives rnstatus-equivalent
// attach/detach/reload over the shared-instance RPC of a Python rnsd using
// the Go rgostatus CLI.
func TestLiveManageRgostatusAgainstPythonRNSD(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath("rnsd"); err != nil {
		t.Skip("rnsd not installed")
	}

	cfgDir := t.TempDir()
	port := freeUDPPort(t)
	ctrlPort := port + 1
	ifacePort := freeUDPPort(t)
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = no",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
		"[interfaces]",
		"  [[mgmt_udp]]",
		"    type = UDPInterface",
		"    enabled = yes",
		"    listen_ip = 127.0.0.1",
		"    listen_port = " + strconv.Itoa(ifacePort),
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rnsd", "--config", cfgDir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start rnsd: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	// Wait for the shared instance RPC to come up.
	cfg, err := rnsutil.LoadConfigDir(cfgDir)
	if err != nil {
		t.Fatalf("load config dir: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		client, err := rnsutil.DialRPC(cfg, nil)
		if err == nil {
			client.SetTimeout(2 * time.Second)
			if _, err := client.GetInterfaceStats(); err == nil {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("python rnsd RPC never came up: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	run := func(args ...string) (int, string) {
		var out bytes.Buffer
		code := cli.RunStatus(append([]string{"-config", cfgDir}, args...),
			cli.Options{Stdout: &out, Stderr: &out})
		return code, out.String()
	}

	code, out := run("-detach", "mgmt_udp")
	if code != 0 || !strings.Contains(out, "detached") {
		t.Fatalf("detach via rgostatus: code=%d out=%q", code, out)
	}
	code, out = run("-attach", "mgmt_udp")
	if code != 0 || !strings.Contains(out, "attached") {
		t.Fatalf("attach via rgostatus: code=%d out=%q", code, out)
	}
	code, out = run("-reload", "mgmt_udp")
	if code != 0 || !strings.Contains(out, "reloaded") {
		t.Fatalf("reload via rgostatus: code=%d out=%q", code, out)
	}
	code, out = run("-detach", "no_such_iface_xyz")
	if code != 1 || !strings.Contains(out, "does not exist") {
		t.Fatalf("detach missing via rgostatus: code=%d out=%q", code, out)
	}
	fmt.Fprintf(os.Stderr, "rgostatus manage interop complete\n")
}
