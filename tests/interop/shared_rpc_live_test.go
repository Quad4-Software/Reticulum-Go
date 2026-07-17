// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live shared-instance RPC interop. Set RUN_LIVE_INTEROP=1.

package interop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
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

func TestLiveInteropPythonRPCAgainstGoUnixSharedInstance(t *testing.T) {
	liveOrSkip(t)
	if runtime.GOOS != "linux" {
		t.Skip("abstract Unix shared-instance sockets are Linux-only")
	}

	name := "gointerop" + strconv.Itoa(freeUDPPort(t))
	cfgDir := t.TempDir()
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = yes",
		"instance_name = " + name,
		"shared_instance_type = unix",
		"",
	}, "\n")
	cfgPath := filepath.Join(cfgDir, "config")
	if err := os.WriteFile(cfgPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	goCfg := &common.ReticulumConfig{
		ShareInstance:      true,
		InstanceName:       name,
		SharedInstanceType: common.SharedInstanceUnix,
		EnableTransport:    false,
		ConfigPath:         cfgPath,
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
		t.Fatalf("python unix rpc probe: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "OK") {
		t.Fatalf("python unix rpc probe failed: %s", out)
	}
}

func TestLiveGoRPCUnixSharedInstance(t *testing.T) {
	liveOrSkip(t)
	if runtime.GOOS != "linux" {
		t.Skip("abstract Unix shared-instance sockets are Linux-only")
	}

	name := "gorpc" + strconv.Itoa(freeUDPPort(t))
	cfgDir := t.TempDir()
	storage := filepath.Join(cfgDir, "storage")
	if err := os.MkdirAll(storage, 0o700); err != nil {
		t.Fatalf("mkdir storage: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config")
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = yes",
		"instance_name = " + name,
		"shared_instance_type = unix",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	goCfg := &common.ReticulumConfig{
		ShareInstance:      true,
		InstanceName:       name,
		SharedInstanceType: common.SharedInstanceUnix,
		EnableTransport:    false,
		ConfigPath:         cfgPath,
	}
	n, err := node.New(goCfg)
	if err != nil {
		t.Fatalf("node new: %v", err)
	}
	if err := n.Start(); err != nil {
		t.Fatalf("node start: %v", err)
	}
	defer n.Stop()

	clientCfg := &common.ReticulumConfig{
		InstanceName:       name,
		SharedInstanceType: common.SharedInstanceUnix,
		ConfigPath:         cfgPath,
	}
	client, err := rnsutil.DialRPC(clientCfg, nil)
	if err != nil {
		t.Fatalf("DialRPC: %v", err)
	}
	if _, err := client.GetLinkCount(); err != nil {
		t.Fatalf("GetLinkCount over unix: %v", err)
	}

	autoCfg := &common.ReticulumConfig{
		InstanceName: name,
		ConfigPath:   cfgPath,
	}
	autoClient, err := rnsutil.DialRPC(autoCfg, nil)
	if err != nil {
		t.Fatalf("DialRPC auto: %v", err)
	}
	if _, err := autoClient.GetLinkCount(); err != nil {
		t.Fatalf("GetLinkCount auto (unix primary): %v", err)
	}
}

