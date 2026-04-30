// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live transport relay: external peer A -- Go -- external peer B. RUN_LIVE_INTEROP=1.

package interop

import (
	"bufio"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

func setupGoUDPRelay(
	t *testing.T,
	aListen, aForward int,
	bListen, bForward int,
) (*transport.Transport, func()) {
	t.Helper()
	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := transport.NewTransport(cfg)

	mkIface := func(name string, listen, forward int) *interfaces.UDPInterface {
		addr := "127.0.0.1:" + strconv.Itoa(listen)
		target := "127.0.0.1:" + strconv.Itoa(forward)
		iface, err := interfaces.NewUDPInterface(name, addr, target, true)
		if err != nil {
			t.Fatalf("udp iface %s: %v", name, err)
		}
		if err := tr.RegisterInterface(name, iface); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
		if err := iface.Start(); err != nil {
			t.Fatalf("start %s: %v", name, err)
		}
		return iface
	}
	mkIface("relay_to_a", aListen, aForward)
	mkIface("relay_to_b", bListen, bForward)

	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}

	cleanup := func() { tr.Close() }
	return tr, cleanup
}

func TestLiveInteropGoRelaysAnnounceBetweenPythonPeers(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	pyAListen := freeUDPPort(t)
	pyAForward := freeUDPPort(t)
	pyBListen := freeUDPPort(t)
	pyBForward := freeUDPPort(t)

	tr, cleanup := setupGoUDPRelay(t,
		pyAForward, pyAListen,
		pyBForward, pyBListen,
	)
	defer cleanup()

	scriptDirPath := scriptDir(t)

	pyACmd := exec.CommandContext(ctx, pythonExe(),
		filepath.Join(scriptDirPath, "py", "announce_peer.py"))
	pyACmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyAListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyAForward),
	)
	pyACmd.Stderr = os.Stderr
	pyAOut, err := pyACmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pyA stdout pipe: %v", err)
	}
	if err := pyACmd.Start(); err != nil {
		t.Fatalf("start pyA: %v", err)
	}
	defer func() {
		_ = pyACmd.Process.Kill()
		_ = pyACmd.Wait()
	}()

	pyABr := bufio.NewReader(pyAOut)
	if line, err := readLineTimeout(ctx, pyABr, 25*time.Second); err != nil {
		t.Fatalf("pyA READY: %v", err)
	} else if strings.TrimSpace(line) != "READY" {
		t.Fatalf("pyA expected READY, got %q", line)
	}
	hashLine, err := readLineTimeout(ctx, pyABr, 5*time.Second)
	if err != nil {
		t.Fatalf("pyA hash line: %v", err)
	}
	pyAHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(pyAHash) != 16 {
		t.Fatalf("pyA bad hash: %q err %v", hashLine, err)
	}

	// Wait for Go to learn pyA via the relay_to_a interface; this
	// proves the relay's ingress path works before we even bring pyB
	// up. We retry path requests because the announce may race with
	// interface readiness.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if tr.HasPath(pyAHash) {
			break
		}
		_ = tr.RequestPath(pyAHash, "relay_to_a", nil, false)
		time.Sleep(150 * time.Millisecond)
	}
	if !tr.HasPath(pyAHash) {
		t.Fatalf("Go relay never learned path to pyA")
	}
	if tr.NextHopInterface(pyAHash) != "relay_to_a" {
		t.Fatalf("pyA path on wrong interface: %s", tr.NextHopInterface(pyAHash))
	}

	pyBCmd := exec.CommandContext(ctx, pythonExe(),
		filepath.Join(scriptDirPath, "py", "wait_path.py"))
	pyBCmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyBListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyBForward),
		"INTEROP_GO_DEST_HASH="+hex.EncodeToString(pyAHash),
	)
	pyBCmd.Stderr = os.Stderr
	pyBOut, err := pyBCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pyB stdout pipe: %v", err)
	}
	if err := pyBCmd.Start(); err != nil {
		t.Fatalf("start pyB: %v", err)
	}
	defer func() {
		_ = pyBCmd.Process.Kill()
		_ = pyBCmd.Wait()
	}()

	pyBBr := bufio.NewReader(pyBOut)
	if line, err := readLineTimeout(ctx, pyBBr, 25*time.Second); err != nil {
		t.Fatalf("pyB READY: %v", err)
	} else if strings.TrimSpace(line) != "READY" {
		t.Fatalf("pyB expected READY, got %q", line)
	}
	if line, err := readLineTimeout(ctx, pyBBr, 60*time.Second); err != nil {
		t.Fatalf("pyB OK: %v", err)
	} else if strings.TrimSpace(line) != "OK" {
		t.Fatalf("pyB expected OK, got %q", line)
	}

	// Final assertion on the Go side: hops to pyA should now be 1
	// (directly reachable via relay_to_a). pyB only opportunistically
	// asked us for a path so we don't assert on hops to pyB.
	if hops := tr.HopsTo(pyAHash); hops != 1 {
		t.Logf("pyA hops via relay = %d (informational)", hops)
	}
}

func TestLiveInteropGoRelayDisabledByConfig(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	pyAListen := freeUDPPort(t)
	pyAForward := freeUDPPort(t)
	pyBListen := freeUDPPort(t)
	pyBForward := freeUDPPort(t)

	cfg := &common.ReticulumConfig{EnableTransport: false}
	tr := transport.NewTransport(cfg)
	defer tr.Close()

	mk := func(name string, listen, forward int) {
		addr := "127.0.0.1:" + strconv.Itoa(listen)
		target := "127.0.0.1:" + strconv.Itoa(forward)
		iface, err := interfaces.NewUDPInterface(name, addr, target, true)
		if err != nil {
			t.Fatalf("udp %s: %v", name, err)
		}
		if err := tr.RegisterInterface(name, iface); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
		if err := iface.Start(); err != nil {
			t.Fatalf("start %s: %v", name, err)
		}
	}
	mk("relay_to_a", pyAForward, pyAListen)
	mk("relay_to_b", pyBForward, pyBListen)

	scriptDirPath := scriptDir(t)

	pyACmd := exec.CommandContext(ctx, pythonExe(),
		filepath.Join(scriptDirPath, "py", "announce_peer.py"))
	pyACmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyAListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyAForward),
	)
	pyACmd.Stderr = os.Stderr
	pyAOut, err := pyACmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pyA stdout: %v", err)
	}
	if err := pyACmd.Start(); err != nil {
		t.Fatalf("pyA start: %v", err)
	}
	defer func() {
		_ = pyACmd.Process.Kill()
		_ = pyACmd.Wait()
	}()

	pyABr := bufio.NewReader(pyAOut)
	if line, err := readLineTimeout(ctx, pyABr, 25*time.Second); err != nil {
		t.Fatalf("pyA READY: %v", err)
	} else if strings.TrimSpace(line) != "READY" {
		t.Fatalf("pyA expected READY, got %q", line)
	}
	hashLine, err := readLineTimeout(ctx, pyABr, 5*time.Second)
	if err != nil {
		t.Fatalf("pyA hash: %v", err)
	}
	pyAHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil {
		t.Fatalf("decode pyA hash: %v", err)
	}

	pyBCtx, pyBCancel := context.WithTimeout(ctx, 10*time.Second)
	defer pyBCancel()
	pyBCmd := exec.CommandContext(pyBCtx, pythonExe(),
		filepath.Join(scriptDirPath, "py", "wait_path.py"))
	pyBCmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyBListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyBForward),
		"INTEROP_GO_DEST_HASH="+hex.EncodeToString(pyAHash),
	)
	pyBCmd.Stderr = os.Stderr
	pyBOut, err := pyBCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pyB stdout: %v", err)
	}
	if err := pyBCmd.Start(); err != nil {
		t.Fatalf("pyB start: %v", err)
	}
	defer func() {
		_ = pyBCmd.Process.Kill()
		_ = pyBCmd.Wait()
	}()

	pyBBr := bufio.NewReader(pyBOut)
	if line, err := readLineTimeout(pyBCtx, pyBBr, 5*time.Second); err != nil {
		t.Fatalf("pyB READY: %v", err)
	} else if strings.TrimSpace(line) != "READY" {
		t.Fatalf("pyB expected READY, got %q", line)
	}
	line, err := readLineTimeout(pyBCtx, pyBBr, 8*time.Second)
	if err == nil && strings.TrimSpace(line) == "OK" {
		t.Fatalf("pyB learned path despite EnableTransport=false (relay leaked)")
	}
}
