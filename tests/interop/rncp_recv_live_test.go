// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live rncp interop, Python sender to Go rgocp listener.
// Set RUN_LIVE_INTEROP=1 to enable.

package interop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestLiveInteropPythonRNCPToGoListener runs a real rgocp listener and has
// the real Python rncp CLI send a file to it.
func TestLiveInteropPythonRNCPToGoListener(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath("rncp"); err != nil {
		t.Skip("rncp not installed")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	goCfg := t.TempDir()
	pyCfg := t.TempDir()
	writeUDPPeerConfig(t, goCfg, portA, portB)
	writeUDPPeerConfig(t, pyCfg, portB, portA)

	rgocp := filepath.Join(repoRoot(t), "bin", "rgocp")
	build := exec.Command("go", "build", "-o", rgocp, "./cmd/rgocp")
	build.Dir = repoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build rgocp: %v\n%s", err, out)
	}

	saveDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
	defer cancel()

	// Learn the destination hash first: rgocp -p prints the identity and the
	// "Listening on <32 hex>" destination hash then exits. The listen
	// invocation below loads the same identity file from the config dir.
	probe := exec.CommandContext(ctx, rgocp, "--config", goCfg, "-p")
	probeOut, err := probe.CombinedOutput()
	if err != nil {
		t.Fatalf("rgocp -p: %v\n%s", err, probeOut)
	}
	hashRe := regexp.MustCompile(`(?m)^.*: ([0-9a-f]{32})\s*$`)
	var destHash string
	for _, m := range hashRe.FindAllStringSubmatch(string(probeOut), -1) {
		destHash = m[1]
	}
	if destHash == "" {
		t.Fatalf("no destination hash in rgocp -p output: %s", probeOut)
	}

	listener := exec.CommandContext(ctx, rgocp,
		"--config", goCfg,
		"-l", "-n", "-s", saveDir, "-O",
	)
	listener.Stdout = os.Stdout
	listener.Stderr = os.Stderr
	if err := listener.Start(); err != nil {
		t.Fatalf("start rgocp listener: %v", err)
	}
	defer func() {
		_ = listener.Process.Kill()
		_ = listener.Wait()
	}()

	// Give the listener a moment to bind, announce, and be discoverable.
	time.Sleep(3 * time.Second)

	payload := strings.Repeat("python-to-go rncp payload\n", 64)
	src := filepath.Join(t.TempDir(), "py_send.txt")
	if err := os.WriteFile(src, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}

	send := exec.CommandContext(ctx, "rncp",
		"--config", pyCfg,
		"-S", "-w", "60",
		src, destHash,
	)
	out, err := send.CombinedOutput()
	if err != nil {
		t.Fatalf("rncp send: %v\n%s", err, out)
	}

	want := filepath.Join(saveDir, "py_send.txt")
	gotDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(gotDeadline) {
		if b, err := os.ReadFile(want); err == nil && string(b) == payload {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	entries, _ := os.ReadDir(saveDir)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	t.Fatalf("received file missing or wrong content; saveDir contents: %v", names)
}
