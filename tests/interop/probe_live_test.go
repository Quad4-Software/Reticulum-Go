// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live probe interop: Go SendProbe against a Python PROVE_ALL destination and
// the real rnprobe CLI against a Go PROVE_ALL destination.
// Set RUN_LIVE_INTEROP=1 to enable.

package interop

import (
	"bufio"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/rnsutil"
)

// startPythonEchoPeer launches rns_echo_peer.py against a pre-written config
// dir and returns the announced destination hash.
func startPythonEchoPeer(t *testing.T, ctx context.Context, cfgDir string, extraEnv ...string) (*exec.Cmd, *bufio.Reader, []byte) {
	t.Helper()
	script := pyScript(t, "rns_echo_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(), "INTEROP_CONFIG_DIR="+cfgDir)
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 30*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("wait READY: %v", err)
	}
	if strings.TrimSpace(line) != "READY" {
		_ = cmd.Process.Kill()
		t.Fatalf("expected READY, got %q", line)
	}
	hashLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("wait hash: %v", err)
	}
	destHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(destHash) != 16 {
		_ = cmd.Process.Kill()
		t.Fatalf("bad peer destination hash: %q", hashLine)
	}
	return cmd, br, destHash
}

// TestLiveInteropGoProbeToPython sends a Go probe packet to a Python
// destination with PROVE_ALL and expects a validated delivery proof.
func TestLiveInteropGoProbeToPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	pyCfg := t.TempDir()
	writeUDPPeerConfig(t, pyCfg, pyListen, pyForward)
	cmd, _, pyHash := startPythonEchoPeer(t, ctx, pyCfg)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	if err := waitPath(ctx, tr, pyHash, 40*time.Second); err != nil {
		t.Fatalf("path to python: %v", err)
	}

	probeCtx, probeCancel := context.WithTimeout(ctx, 45*time.Second)
	defer probeCancel()
	res, err := rnsutil.SendProbe(probeCtx, tr, pyHash, 32)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !res.Delivered {
		t.Fatal("probe not proved by python destination")
	}
}

// TestLiveInteropPythonRnprobeToGo runs the real rnprobe CLI against a Go
// destination with ProveAll.
func TestLiveInteropPythonRnprobeToGo(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath("rnprobe"); err != nil {
		t.Skip("rnprobe not installed")
	}

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	idGo, err := identity.New()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	destGo, err := destination.New(idGo, destination.In, destination.Single, interopApp, tr, interopAspect)
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	destGo.SetProofStrategy(destination.ProveAll)
	destGo.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {})
	if err := destGo.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	pyCfg := t.TempDir()
	writeUDPPeerConfig(t, pyCfg, pyListen, pyForward)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rnprobe",
		"--config", pyCfg,
		"-t", "30",
		interopApp+"."+interopAspect,
		hex.EncodeToString(destGo.GetHash()),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rnprobe: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Valid reply") {
		t.Fatalf("rnprobe reported no valid reply: %s", out)
	}
}
