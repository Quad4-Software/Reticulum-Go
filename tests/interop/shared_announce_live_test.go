// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live interop: Python shared-instance clients and a Go owner with
// enable_transport = no. Set RUN_LIVE_INTEROP=1. Requires python3 + RNS.

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

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	"github.com/Quad4-Software/Reticulum-Go/pkg/node"
)

func startGoSharedOwner(t *testing.T, enableTransport bool, sharedPort, ctrlPort, goUDP, peerUDP int) (*node.Node, string, func()) {
	t.Helper()
	cfgDir := t.TempDir()
	transportLine := "enable_transport = no"
	if enableTransport {
		transportLine = "enable_transport = yes"
	}
	config := strings.Join([]string{
		"[reticulum]",
		transportLine,
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

	goCfg := &common.ReticulumConfig{
		EnableTransport:     enableTransport,
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

	udpIface, err := interfaces.NewUDPInterface(
		"interop_udp",
		fmt.Sprintf("127.0.0.1:%d", goUDP),
		fmt.Sprintf("127.0.0.1:%d", peerUDP),
		true,
	)
	if err != nil {
		n.Stop()
		t.Fatalf("udp iface: %v", err)
	}
	if err := n.Transport().RegisterInterface("interop_udp", udpIface); err != nil {
		n.Stop()
		t.Fatalf("register udp: %v", err)
	}
	if err := udpIface.Start(); err != nil {
		n.Stop()
		t.Fatalf("start udp: %v", err)
	}

	cleanup := func() {
		_ = udpIface.Stop()
		n.Stop()
	}
	return n, cfgDir, cleanup
}

// TestLiveInteropSharedClientReceivesAnnounceTransportDisabled checks that a
// Python shared-instance client sees a UDP peer announce through a Go owner
// with enable_transport = no, without sending a path request.
func TestLiveInteropSharedClientReceivesAnnounceTransportDisabled(t *testing.T) {
	liveOrSkip(t)

	peerListen := freeUDPPort(t)
	goUDPListen := freeUDPPort(t)
	sharedPort := freeUDPPort(t)
	ctrlPort := freeUDPPort(t)

	_, cfgDir, cleanup := startGoSharedOwner(t, false, sharedPort, ctrlPort, goUDPListen, peerListen)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	peerCmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "announce_peer.py"))
	peerCmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(peerListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(goUDPListen),
		"INTEROP_ANNOUNCE_REPEAT_SEC=2",
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

	clientCmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "shared_client_wait_announce.py"))
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

	clientBR := bufio.NewReader(clientOut)
	connectedLine, err := readLineTimeout(ctx, clientBR, 15*time.Second)
	if err != nil {
		t.Fatalf("wait client CONNECTED: %v", err)
	}
	if !strings.Contains(connectedLine, "CONNECTED") {
		t.Fatalf("expected CONNECTED, got %q", connectedLine)
	}

	waitingLine, err := readLineTimeout(ctx, clientBR, 10*time.Second)
	if err != nil {
		t.Fatalf("wait client WAITING: %v", err)
	}
	t.Logf("Client: %s", strings.TrimSpace(waitingLine))

	resultLine, err := readLineTimeout(ctx, clientBR, 70*time.Second)
	if err != nil {
		t.Fatalf("wait client result: %v", err)
	}
	result := strings.TrimSpace(resultLine)
	t.Logf("Client result: %s", result)
	if !strings.HasPrefix(result, "PATH_FOUND") {
		t.Fatalf("Python client did not see copied announce: %s", result)
	}
}

// TestLiveInteropSharedClientAnnounceReachesWANTransportDisabled checks that
// a Python shared-instance client announce reaches a UDP peer through a Go
// owner with enable_transport = no.
func TestLiveInteropSharedClientAnnounceReachesWANTransportDisabled(t *testing.T) {
	liveOrSkip(t)

	peerListen := freeUDPPort(t)
	goUDPListen := freeUDPPort(t)
	sharedPort := freeUDPPort(t)
	ctrlPort := freeUDPPort(t)

	_, cfgDir, cleanup := startGoSharedOwner(t, false, sharedPort, ctrlPort, goUDPListen, peerListen)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	clientCmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "shared_client_announce.py"))
	clientCmd.Env = append(os.Environ(), "INTEROP_CONFIG_DIR="+cfgDir)
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

	clientBR := bufio.NewReader(clientOut)
	connectedLine, err := readLineTimeout(ctx, clientBR, 15*time.Second)
	if err != nil {
		t.Fatalf("wait client CONNECTED: %v", err)
	}
	if !strings.Contains(connectedLine, "CONNECTED") {
		t.Fatalf("expected CONNECTED, got %q", connectedLine)
	}
	hashLine, err := readLineTimeout(ctx, clientBR, 10*time.Second)
	if err != nil {
		t.Fatalf("wait client hash: %v", err)
	}
	clientHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(clientHash) != 16 {
		t.Fatalf("bad client hash: %q err %v", hashLine, err)
	}
	announcedLine, err := readLineTimeout(ctx, clientBR, 10*time.Second)
	if err != nil {
		t.Fatalf("wait ANNOUNCED: %v", err)
	}
	if strings.TrimSpace(announcedLine) != "ANNOUNCED" {
		t.Fatalf("expected ANNOUNCED, got %q", announcedLine)
	}

	peerCmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "wait_announce.py"))
	peerCmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(peerListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(goUDPListen),
		"INTEROP_DEST_HASH="+hex.EncodeToString(clientHash),
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

	peerBR := bufio.NewReader(peerOut)
	readyLine, err := readLineTimeout(ctx, peerBR, 25*time.Second)
	if err != nil {
		t.Fatalf("wait peer READY: %v", err)
	}
	if strings.TrimSpace(readyLine) != "READY" {
		t.Fatalf("expected READY, got %q", readyLine)
	}

	resultLine, err := readLineTimeout(ctx, peerBR, 45*time.Second)
	if err != nil {
		t.Fatalf("wait peer result: %v", err)
	}
	result := strings.TrimSpace(resultLine)
	t.Logf("WAN peer result: %s", result)
	if !strings.HasPrefix(result, "OK") {
		t.Fatalf("WAN peer did not see client announce: %s", result)
	}
}
