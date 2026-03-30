// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io

package interop

import (
	"bufio"
	"context"
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

func freeUDPPort(t *testing.T) int {
	t.Helper()
	a, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve udp: %v", err)
	}
	c, err := net.ListenUDP("udp", a)
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func pythonExe() string {
	if p := os.Getenv("PYTHON_INTEROP"); p != "" {
		return p
	}
	return "python3"
}

// TestLiveInteropPythonSeesGoAnnouncePath runs a Python RNS peer against this stack over UDP on loopback.
// Requires the `rns` package (pip) or RETICULUM_PATH pointing at the Python Reticulum tree.
// Enable with: RUN_LIVE_INTEROP=1
func TestLiveInteropPythonSeesGoAnnouncePath(t *testing.T) {
	if os.Getenv("RUN_LIVE_INTEROP") != "1" {
		t.Skip("set RUN_LIVE_INTEROP=1 to run live Python-Go interop")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	if pyListen == pyForward {
		t.Fatal("expected distinct UDP ports")
	}

	cfg := &common.ReticulumConfig{}
	tr := transport.NewTransport(cfg)
	defer tr.Close()

	idGo, err := identity.New()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}

	destGo, err := destination.New(idGo, destination.IN, destination.SINGLE, "interop_live", tr, "peer")
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	destGo.AcceptsLinks(true)

	addrGo := "127.0.0.1:" + strconv.Itoa(pyForward)
	targetGo := "127.0.0.1:" + strconv.Itoa(pyListen)
	iface, err := interfaces.NewUDPInterface("interop_udp", addrGo, targetGo, true)
	if err != nil {
		t.Fatalf("udp iface: %v", err)
	}
	if err := tr.RegisterInterface("interop_udp", iface); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := iface.Start(); err != nil {
		t.Fatalf("start iface: %v", err)
	}
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	script := filepath.Join(filepath.Dir(testFile), "python_interop_wait_path.py")

	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_GO_DEST_HASH="+hex.EncodeToString(destGo.GetHash()),
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
	line, err := readLineTimeout(ctx, br, 20*time.Second)
	if err != nil {
		t.Fatalf("wait READY: %v", err)
	}
	if strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q", line)
	}

	if err := destGo.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	line, err = readLineTimeout(ctx, br, 45*time.Second)
	if err != nil {
		t.Fatalf("wait OK: %v", err)
	}
	if strings.TrimSpace(line) != "OK" {
		t.Fatalf("expected OK, got %q", line)
	}
}

func readLineTimeout(ctx context.Context, br *bufio.Reader, d time.Duration) (string, error) {
	type res struct {
		s   string
		err error
	}
	ch := make(chan res, 1)
	go func() {
		s, err := br.ReadString('\n')
		ch <- res{s, err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-ch:
		return r.s, r.err
	case <-time.After(d):
		return "", context.DeadlineExceeded
	}
}
