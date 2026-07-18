// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live I2P Go and Python interop. Requires RUN_LIVE_INTEROP=1 and a local SAM
// bridge (also checked via RUN_LIVE_I2P-style SAM reachability).

package interop

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/i2p"
	"quad4/reticulum-go/pkg/interfaces"
)

func liveI2PInteropOrSkip(t *testing.T) {
	t.Helper()
	liveOrSkip(t)
	addr := i2p.SAMAddressFromEnv()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Skipf("SAM not reachable at %s: %v", addr, err)
	}
	_ = conn.Close()
}

// waitStdoutToken reads lines until one equals token or has the prefix (when
// token ends with "="), skipping RNS log noise on stdout.
func waitStdoutToken(ctx context.Context, br *bufio.Reader, token string, d time.Duration) (string, error) {
	deadline := time.Now().Add(d)
	prefix := strings.HasSuffix(token, "=")
	for time.Now().Before(deadline) {
		remain := time.Until(deadline)
		if remain <= 0 {
			break
		}
		line, err := readLineTimeout(ctx, br, remain)
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if prefix {
			if strings.HasPrefix(line, token) {
				return line, nil
			}
			continue
		}
		if line == token {
			return line, nil
		}
	}
	return "", context.DeadlineExceeded
}

func TestLiveInteropI2PGoServerPythonClient(t *testing.T) {
	liveI2PInteropOrSkip(t)
	dir := t.TempDir()
	storage := filepath.Join(dir, "storage")

	iface, err := interfaces.NewI2PInterface("go_i2p", &common.InterfaceConfig{
		Type:           "I2PInterface",
		Enabled:        true,
		I2PConnectable: true,
	}, &interfaces.FromConfigContext{
		I2PStoragePath: storage,
		TransportID:    []byte("interop-go-i2p-server"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := iface.Start(); err != nil {
		t.Fatal(err)
	}
	defer iface.Stop()

	deadline := time.Now().Add(3 * time.Minute)
	for iface.Base32() == "" && time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
	}
	if iface.Base32() == "" {
		t.Fatal("Go connectable interface never published b32")
	}
	peerDest := iface.Base32() + ".b32.i2p"
	t.Logf("Go published %s", peerDest)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pyCfg := filepath.Join(dir, "py_client")
	if err := os.MkdirAll(pyCfg, 0o750); err != nil {
		t.Fatal(err)
	}
	script := pyScript(t, "i2p_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_I2P_MODE=client",
		"INTEROP_I2P_PEER="+peerDest,
		"INTEROP_CONFIG_DIR="+pyCfg,
		"INTEROP_I2P_TIMEOUT=180",
	)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	line, err := waitStdoutToken(ctx, br, "ONLINE", 3*time.Minute)
	if err != nil {
		t.Fatalf("waiting for Python ONLINE: %v", err)
	}
	if line != "ONLINE" {
		t.Fatalf("expected ONLINE got %q", line)
	}

	peerDeadline := time.Now().Add(90 * time.Second)
	for iface.Clients() < 1 && time.Now().Before(peerDeadline) {
		time.Sleep(500 * time.Millisecond)
	}
	if iface.Clients() < 1 {
		t.Fatal("Go server never saw inbound I2P peer from Python")
	}
}

func TestLiveInteropI2PPythonServerGoClient(t *testing.T) {
	liveI2PInteropOrSkip(t)
	dir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pyCfg := filepath.Join(dir, "py_server")
	if err := os.MkdirAll(pyCfg, 0o750); err != nil {
		t.Fatal(err)
	}
	script := pyScript(t, "i2p_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_I2P_MODE=server",
		"INTEROP_CONFIG_DIR="+pyCfg,
		"INTEROP_I2P_TIMEOUT=180",
	)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	line, err := waitStdoutToken(ctx, br, "B32=", 3*time.Minute)
	if err != nil {
		t.Fatalf("waiting for Python B32: %v", err)
	}
	if !strings.HasPrefix(line, "B32=") {
		t.Fatalf("expected B32= got %q", line)
	}
	b32 := strings.TrimPrefix(line, "B32=")
	if len(b32) != 52 {
		t.Fatalf("unexpected b32 length %d: %q", len(b32), b32)
	}
	peerDest := b32 + ".b32.i2p"
	t.Logf("Python published %s", peerDest)

	storage := filepath.Join(dir, "go_storage")
	parent, err := interfaces.NewI2PInterface("go_i2p_client", &common.InterfaceConfig{
		Type:    "I2PInterface",
		Enabled: true,
	}, &interfaces.FromConfigContext{
		I2PStoragePath: storage,
		TransportID:    []byte("interop-go-i2p-client"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	defer parent.Stop()

	peer := interfaces.NewI2PInterfacePeer(parent, "go_i2p_client to "+peerDest, peerDest, 3, parent.InterfaceConfig())
	defer peer.Stop()

	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		if peer.IsOnline() {
			t.Log("Go client peer online")
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("Go client never online last_error=%q", peer.LastError())
}
