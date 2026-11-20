// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live shared-instance RPC interop. Set RUN_LIVE_INTEROP=1.

package interop

import (
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/node"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/rnsutil"
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

// TestLiveInteropKnownDestinationsOverRPC verifies Python 1.6.1 split-mode
// clients fall back to the get=known_destinations RPC when the shared
// instance's database is not directly readable (client has its own config
// dir), and that the Go server's response decodes on the Python side.
func TestLiveInteropKnownDestinationsOverRPC(t *testing.T) {
	liveOrSkip(t)

	port := freeUDPPort(t)
	ctrlPort := freeUDPPort(t)

	// Server config dir. The Go node writes its known destinations into its
	// own storage so the client cannot see the database file (split mode).
	serverCfgDir := t.TempDir()
	serverCfg := strings.Join([]string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
	}, "\n")
	cfgPath := filepath.Join(serverCfgDir, "config")
	if err := os.WriteFile(cfgPath, []byte(serverCfg), 0o600); err != nil {
		t.Fatalf("write server config: %v", err)
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

	// Seed a known destination on the Go server.
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destHash := destination.Hash(id, "interop_kd", "probe")
	if !identity.RememberIdentity([]byte("pkthashpkt-hash1"), destHash, id.GetPublicKey(), []byte("kd-app-data"), id) {
		t.Fatal("server did not remember destination")
	}

	// Split-mode client: separate config dir so the server's
	// known_destinations.db is not on its storage path. A different storage
	// means a different transport identity, so the RPC authkey must be set
	// explicitly, same as a real split-mode deployment.
	clientCfgDir := t.TempDir()
	rpcKey := hex.EncodeToString(n.Transport().RPCAuthKey())
	clientCfg := strings.Join([]string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"rpc_key = " + rpcKey,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(clientCfgDir, "config"), []byte(clientCfg), 0o600); err != nil {
		t.Fatalf("write client config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "kd_probe.py"))
	cmd.Env = append(os.Environ(), "INTEROP_CONFIG_DIR="+clientCfgDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python kd probe: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "OK") {
		t.Fatalf("python kd probe failed: %s", out)
	}
	want := "KD_KEY " + hex.EncodeToString(destHash)
	if !strings.Contains(string(out), want) {
		t.Fatalf("kd probe missing destination key %s: %s", want, out)
	}
}
