// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live IFAC UDP loopback interop. RUN_LIVE_INTEROP=1.

package interop

import (
	"bufio"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/ifac"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/transport"
)

const (
	ifacInteropApp     = "interop_pygo"
	ifacInteropAspect  = "ifacsvc"
	ifacInteropNetname = "ifac-interop"
	ifacInteropNetkey  = "shared-secret"
	ifacInteropSize    = 16
)

func setupGoIFACPeer(t *testing.T, pyListen, pyForward int, netname, netkey string, size int) (*transport.Transport, *interfaces.UDPInterface, func()) {
	t.Helper()
	cfg := &common.ReticulumConfig{}
	tr := transport.NewTransport(cfg)
	addrGo := "127.0.0.1:" + strconv.Itoa(pyForward)
	targetGo := "127.0.0.1:" + strconv.Itoa(pyListen)
	iface, err := interfaces.NewUDPInterface("interop_ifac_udp", addrGo, targetGo, true)
	if err != nil {
		t.Fatalf("udp iface: %v", err)
	}
	if netname != "" || netkey != "" {
		id, err := ifac.New(size, netname, netkey)
		if err != nil {
			t.Fatalf("ifac identity: %v", err)
		}
		iface.SetIFAC(id)
	}
	if err := tr.RegisterInterface("interop_ifac_udp", iface); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := iface.Start(); err != nil {
		t.Fatalf("start iface: %v", err)
	}
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}
	cleanup := func() { tr.Close() }
	return tr, iface, cleanup
}

// Verifies Go decodes an IFAC-masked announce from the peer and learns the
// path.
func TestLiveInteropIFACGoSeesPythonAnnounce(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoIFACPeer(t, pyListen, pyForward, ifacInteropNetname, ifacInteropNetkey, ifacInteropSize)
	defer cleanup()

	script := pyScript(t, "ifac_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_NETNAME="+ifacInteropNetname,
		"INTEROP_NETKEY="+ifacInteropNetkey,
		"INTEROP_IFAC_SIZE="+strconv.Itoa(ifacInteropSize),
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
	hashLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("wait hash: %v", err)
	}
	pyHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(pyHash) != 16 {
		t.Fatalf("bad python destination hash: %q err %v", hashLine, err)
	}
	if err := waitPath(ctx, tr, pyHash, 60*time.Second); err != nil {
		t.Fatalf("Go never learned path to Python destination over IFAC: %v", err)
	}
}

// Verifies the peer decodes an IFAC-masked announce from Go and learns the
// path.
func TestLiveInteropIFACPythonSeesGoAnnounce(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoIFACPeer(t, pyListen, pyForward, ifacInteropNetname, ifacInteropNetkey, ifacInteropSize)
	defer cleanup()

	idGo, err := identity.New()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	destGo, err := destination.New(idGo, destination.In, destination.Single, ifacInteropApp, tr, ifacInteropAspect)
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	destGo.AcceptsLinks(true)

	script := pyScript(t, "ifac_wait_path.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_NETNAME="+ifacInteropNetname,
		"INTEROP_NETKEY="+ifacInteropNetkey,
		"INTEROP_IFAC_SIZE="+strconv.Itoa(ifacInteropSize),
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
	line, err := readLineTimeout(ctx, br, 25*time.Second)
	if err != nil {
		t.Fatalf("wait READY: %v", err)
	}
	if strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q", line)
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if err := destGo.Announce(false, nil, nil); err != nil {
			t.Fatalf("announce: %v", err)
		}
		readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
		line, err := readLineTimeout(readCtx, br, 5*time.Second)
		readCancel()
		if err == nil && strings.TrimSpace(line) == "OK" {
			return
		}
	}
	t.Fatalf("Python never learned path to Go destination over IFAC")
}

// TestLiveInteropIFACGoRejectsUnauthenticated verifies that a Go peer with
// IFAC enabled drops announces from a peer that does not configure the same
// network_name/passphrase.
func TestLiveInteropIFACGoRejectsUnauthenticated(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoIFACPeer(t, pyListen, pyForward, ifacInteropNetname, ifacInteropNetkey, ifacInteropSize)
	defer cleanup()

	script := pyScript(t, "announce_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
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
	hashLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("wait hash: %v", err)
	}
	pyHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(pyHash) != 16 {
		t.Fatalf("bad python destination hash: %q err %v", hashLine, err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if tr.HasPath(pyHash) {
			t.Fatalf("Go peer with IFAC enabled accepted unauthenticated announce from %x", pyHash)
		}
		time.Sleep(150 * time.Millisecond)
	}
}
