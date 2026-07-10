// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live interop: verifies that a Python shared-instance client's path request
// is forwarded by the Go shared-instance server onto its UDP interface.
// This exercises the is_from_local_client branch in processPathRequest.
//
// Set RUN_LIVE_INTEROP=1 to enable. Requires python3 + RNS installed.

package interop

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/node"
)

// TestLiveInteropSharedClientPathRequestForwarding sets up:
//   - Go shared-instance server with a UDP interface to a Python announce peer
//   - Python announce peer (announces a destination on UDP)
//   - Python shared-instance client (connects to Go server, requests path)
//
// The Go server must forward the Python client's path request onto its UDP
// interface (the is_from_local_client branch). The announce peer receives the
// PR, announces, and the path gets resolved.
func TestLiveInteropSharedClientPathRequestForwarding(t *testing.T) {
	liveOrSkip(t)

	peerListen := freeUDPPort(t)  // Python announce peer listens here
	goUDPListen := freeUDPPort(t) // Go UDP interface listens here
	sharedPort := freeUDPPort(t)
	ctrlPort := freeUDPPort(t)

	cfgDir := t.TempDir()
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = yes",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(sharedPort),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
		"[logging]",
		"loglevel = 3",
		"",
		"[interfaces]",
		"",
	}, "\n")
	cfgPath := filepath.Join(cfgDir, "config")
	if err := os.WriteFile(cfgPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// --- Start Go shared-instance server ---
	goCfg := &common.ReticulumConfig{
		EnableTransport:     true,
		ShareInstance:       true,
		SharedInstancePort:  sharedPort,
		InstanceControlPort: ctrlPort,
		SharedInstanceType:  common.SharedInstanceTCP,
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

	// Add a UDP interface to the Go node pointing at the Python announce peer.
	udpIface, err := interfaces.NewUDPInterface(
		"interop_udp",
		fmt.Sprintf("127.0.0.1:%d", goUDPListen),
		fmt.Sprintf("127.0.0.1:%d", peerListen),
		true,
	)
	if err != nil {
		t.Fatalf("udp iface: %v", err)
	}
	if err := n.Transport().RegisterInterface("interop_udp", udpIface); err != nil {
		t.Fatalf("register udp: %v", err)
	}
	if err := udpIface.Start(); err != nil {
		t.Fatalf("start udp: %v", err)
	}
	defer udpIface.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// --- Start Python announce peer ---
	peerCmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "announce_peer.py"))
	peerCmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(peerListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(goUDPListen),
	)
	peerCmd.Stderr = os.Stderr
	peerOut, err := peerCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("peer stdout pipe: %v", err)
	}
	if err := peerCmd.Start(); err != nil {
		t.Fatalf("start peer: %v", err)
	}
	defer func() {
		_ = peerCmd.Process.Kill()
		_ = peerCmd.Wait()
	}()

	// Wait for the peer to print its destination hash.
	peerBR := bufio.NewReader(peerOut)
	line, err := readLineTimeout(ctx, peerBR, 25*time.Second)
	if err != nil {
		t.Fatalf("wait peer READY: %v", err)
	}
	if strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q", line)
	}
	hashLine, err := readLineTimeout(ctx, peerBR, 5*time.Second)
	if err != nil {
		t.Fatalf("wait peer hash: %v", err)
	}
	peerHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(peerHash) != 16 {
		t.Fatalf("bad peer hash: %q err %v", hashLine, err)
	}
	t.Logf("Python announce peer hash: %x", peerHash)

	// Wait for the announce to propagate so the Go server knows the path.
	// This tests the "known path" branch for local-client PRs.
	time.Sleep(5 * time.Second)

	if !n.Transport().HasPath(peerHash) {
		t.Fatalf("Go server should have received announce from peer by now")
	}
	t.Logf("Go server has path to peer (hops=%d)", n.Transport().HopsTo(peerHash))

	// --- Start Python shared-instance client that requests the path ---
	clientCmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "shared_client_path_request.py"))
	clientCmd.Env = append(os.Environ(),
		"INTEROP_CONFIG_DIR="+cfgDir,
		"INTEROP_PEER_HASH="+hex.EncodeToString(peerHash),
	)
	clientCmd.Stderr = os.Stderr
	clientOut, err := clientCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("client stdout pipe: %v", err)
	}
	if err := clientCmd.Start(); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() {
		_ = clientCmd.Process.Kill()
		_ = clientCmd.Wait()
	}()

	// Read client output line by line.
	clientBR := bufio.NewReader(clientOut)

	connectedLine, err := readLineTimeout(ctx, clientBR, 15*time.Second)
	if err != nil {
		t.Fatalf("wait client CONNECTED: %v", err)
	}
	if !strings.Contains(connectedLine, "CONNECTED") {
		t.Fatalf("expected CONNECTED, got %q", connectedLine)
	}
	t.Logf("Python shared-instance client connected to Go server")

	requestingLine, err := readLineTimeout(ctx, clientBR, 10*time.Second)
	if err != nil {
		t.Fatalf("wait client REQUESTING: %v", err)
	}
	t.Logf("Client: %s", strings.TrimSpace(requestingLine))

	// Wait for the path resolution result.
	// The Go server processes the PR from the local client and answers with
	// the known path (creating an announce table entry). The announce table
	// processing delivers the announce back to the Python client through the
	// shared instance, allowing the client to see the path.
	resultLine, err := readLineTimeout(ctx, clientBR, 70*time.Second)
	if err != nil {
		t.Fatalf("wait client result: %v", err)
	}
	result := strings.TrimSpace(resultLine)
	t.Logf("Client result: %s", result)

	if result != "PATH_FOUND" {
		t.Fatalf("path not found via shared instance: %s", result)
	}

	t.Logf("SUCCESS: Python shared-instance client found path through Go server")
}
