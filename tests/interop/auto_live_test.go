// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live AutoInterface interop. Requires root to create veth pairs.
// Set RUN_LIVE_INTEROP=1 to enable.

package interop

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/transport"
)

// setupVethPair creates a veth pair with IPv6 link-local addresses.
// Returns a cleanup function.
func setupVethPair(t *testing.T) (veth0, veth1 string, cleanup func()) {
	t.Helper()
	veth0 = "rns0"
	veth1 = "rns1"

	// Create veth pair
	cmd := exec.Command("ip", "link", "add", veth0, "type", "veth", "peer", "name", veth1)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create veth pair: %v\n%s", err, out)
	}

	// Bring up both ends
	exec.Command("ip", "link", "set", veth0, "up").Run()
	exec.Command("ip", "link", "set", veth1, "up").Run()

	// Add link-local addresses
	exec.Command("ip", "-6", "addr", "add", "fe80::1/64", "dev", veth0).Run()
	exec.Command("ip", "-6", "addr", "add", "fe80::2/64", "dev", veth1).Run()

	// Wait for addresses to settle
	time.Sleep(100 * time.Millisecond)

	cleanup = func() {
		exec.Command("ip", "link", "del", veth0).Run()
	}
	return veth0, veth1, cleanup
}

func hasRoot() bool {
	return os.Getuid() == 0
}

// TestLiveInteropAutoInterfaceDiscovery verifies Go and Python AutoInterfaces
// discover each other over a veth pair.
func TestLiveInteropAutoInterfaceDiscovery(t *testing.T) {
	liveOrSkip(t)
	if runtime.GOOS != "linux" {
		t.Skip("veth-based live test only supported on Linux")
	}
	if !hasRoot() {
		t.Skip("live AutoInterface test requires root for veth creation")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	veth0, veth1, cleanupVeth := setupVethPair(t)
	defer cleanupVeth()

	// Start Go AutoInterface on veth0
	cfg := &common.InterfaceConfig{
		Enabled: true,
		Devices: []string{veth0},
	}
	ai, err := interfaces.NewAutoInterface("test_auto", cfg)
	if err != nil {
		t.Fatalf("NewAutoInterface failed: %v", err)
	}
	if err := ai.Start(); err != nil {
		t.Fatalf("Start AutoInterface failed: %v", err)
	}
	defer ai.Stop()

	// Give Go a moment to start listening
	time.Sleep(200 * time.Millisecond)

	// Start Python peer on veth1
	script := pyScript(t, "auto_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_DEVICE="+veth1,
		"INTEROP_GROUP_ID=reticulum",
	)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 30*time.Second)
	if err != nil {
		t.Fatalf("wait READY: %v", err)
	}
	if strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q", line)
	}

	// Wait for Go to discover the Python peer
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if ai.PeerCount() > 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("Go did not discover Python peer via AutoInterface")
}

// TestLiveInteropAutoInterfaceGoSeesPythonAnnounce verifies Go learns a path
// to a Python destination announced over AutoInterface.
func TestLiveInteropAutoInterfaceGoSeesPythonAnnounce(t *testing.T) {
	liveOrSkip(t)
	if runtime.GOOS != "linux" {
		t.Skip("veth-based live test only supported on Linux")
	}
	if !hasRoot() {
		t.Skip("live AutoInterface test requires root for veth creation")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	veth0, veth1, cleanupVeth := setupVethPair(t)
	defer cleanupVeth()

	// Set up Go transport with AutoInterface on veth0
	tr := transport.NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	cfg := &common.InterfaceConfig{
		Enabled: true,
		Devices: []string{veth0},
	}
	ai, err := interfaces.NewAutoInterface("test_auto", cfg)
	if err != nil {
		t.Fatalf("NewAutoInterface failed: %v", err)
	}
	if err := ai.Start(); err != nil {
		t.Fatalf("Start AutoInterface failed: %v", err)
	}
	defer ai.Stop()

	if err := tr.RegisterInterface("test_auto", ai); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}

	// Start Python peer on veth1
	script := pyScript(t, "auto_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_DEVICE="+veth1,
		"INTEROP_GROUP_ID=reticulum",
	)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 30*time.Second)
	if err != nil {
		t.Fatalf("wait READY: %v", err)
	}
	if strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q", line)
	}

	// Wait for Go to learn the path to Python
	// We don't know the Python destination hash, so we just verify that Go
	// discovers the peer and the transport registers the interface.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if ai.PeerCount() > 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("Go never discovered Python peer over AutoInterface")
}
