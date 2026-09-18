// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live Go-Go smoke tests for the Go-only utilities: rgospeed over a real UDP
// pair and rgosnap against a real Go shared-instance daemon.
// Set RUN_LIVE_INTEROP=1 to enable.

//go:build !js

package interop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/node"
)

func buildTool(t *testing.T, name string) string {
	t.Helper()
	bin := filepath.Join(repoRoot(t), "bin", name)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/"+name)
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
	return bin
}

// TestLiveRgospeedLoopback runs the in-process loopback speedtest.
func TestLiveRgospeedLoopback(t *testing.T) {
	liveOrSkip(t)
	bin := buildTool(t, "rgospeed")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-loopback", "-bytes", "65536", "-json").CombinedOutput()
	if err != nil {
		t.Fatalf("rgospeed -loopback: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "ok=true") {
		t.Fatalf("loopback result missing ok=true: %s", out)
	}
}

// TestLiveRgospeedUDPPair runs a real rgospeed listener and client over a UDP
// interface pair: announce, link establishment, and a bulk transfer.
func TestLiveRgospeedUDPPair(t *testing.T) {
	liveOrSkip(t)
	bin := buildTool(t, "rgospeed")

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	cfgA := t.TempDir()
	cfgB := t.TempDir()
	writeUDPPeerConfig(t, cfgA, portA, portB)
	writeUDPPeerConfig(t, cfgB, portB, portA)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Discover the listener destination hash via -p (same config dir means the
	// listener loads the same identity).
	probe := exec.CommandContext(ctx, bin, "--config", cfgA, "-p")
	probeOut, err := probe.CombinedOutput()
	if err != nil {
		t.Fatalf("rgospeed -p: %v\n%s", err, probeOut)
	}
	hashRe := regexp.MustCompile(`(?m)([0-9a-f]{32})`)
	var destHash string
	for _, line := range strings.Split(string(probeOut), "\n") {
		if m := hashRe.FindStringSubmatch(line); m != nil {
			destHash = m[1]
		}
	}
	if destHash == "" {
		t.Fatalf("no destination hash in rgospeed -p output: %s", probeOut)
	}

	listener := exec.CommandContext(ctx, bin,
		"--config", cfgA,
		"-l", "-announce", "2",
	)
	listener.Stdout = os.Stdout
	listener.Stderr = os.Stderr
	if err := listener.Start(); err != nil {
		t.Fatalf("start rgospeed listener: %v", err)
	}
	defer func() {
		_ = listener.Process.Kill()
		_ = listener.Wait()
	}()

	// Wait for the listener to announce at least once.
	time.Sleep(4 * time.Second)

	client := exec.CommandContext(ctx, bin,
		"--config", cfgB,
		"-bytes", "262144",
		"-timeout", "120",
		"-json",
		destHash,
	)
	out, err := client.CombinedOutput()
	if err != nil {
		t.Fatalf("rgospeed client: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "ok=true") {
		t.Fatalf("client result missing ok=true: %s", out)
	}
}

// TestLiveRgosnapGoDaemon runs rgosnap against a live Go shared-instance
// daemon over TCP RPC.
func TestLiveRgosnapGoDaemon(t *testing.T) {
	liveOrSkip(t)
	bin := buildTool(t, "rgosnap")

	cfgDir := t.TempDir()
	port := freeUDPPort(t)
	ctrlPort := freeUDPPort(t)
	cfgPath := filepath.Join(cfgDir, "config")
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = yes",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	n, err := node.New(&common.ReticulumConfig{
		EnableTransport:     true,
		ShareInstance:       true,
		SharedInstancePort:  port,
		InstanceControlPort: ctrlPort,
		SharedInstanceType:  common.SharedInstanceTCP,
		ConfigPath:          cfgPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--config", cfgDir).CombinedOutput()
	if err != nil {
		t.Fatalf("rgosnap: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "path_count") || !strings.Contains(string(out), "transport_id") {
		t.Fatalf("rgosnap output missing expected keys: %s", out)
	}
}
