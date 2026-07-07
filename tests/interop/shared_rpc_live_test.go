// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live shared-instance RPC interop. Set RUN_LIVE_INTEROP=1.

package interop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/node"
)

func TestLiveInteropPythonRPCAgainstGoSharedInstance(t *testing.T) {
	liveOrSkip(t)

	port := freeUDPPort(t)
	ctrlPort := port + 1
	cfgDir := t.TempDir()
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
	}, "\n")
	cfgPath := filepath.Join(cfgDir, "config")
	if err := os.WriteFile(cfgPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	goCfg := &common.ReticulumConfig{
		ShareInstance:       true,
		SharedInstancePort:  port,
		InstanceControlPort: ctrlPort,
		SharedInstanceType:  common.SharedInstanceTCP,
		EnableTransport:     false,
		ConfigPath:          cfgPath,
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

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "rpc_probe.py"))
	cmd.Env = append(os.Environ(), "INTEROP_CONFIG_DIR="+cfgDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python rpc probe: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "OK") {
		t.Fatalf("python rpc probe failed: %s", out)
	}
}
