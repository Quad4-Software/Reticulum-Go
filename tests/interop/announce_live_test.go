// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

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

	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

// TestLiveInteropAnnouncePythonNoRatchetToGo verifies Python announces that omit
// the ratchet field are accepted by pkg/announce.HandleAnnounce and learn a
// live path on the Go transport.
func TestLiveInteropAnnouncePythonNoRatchetToGo(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	script := pyScript(t, "announce_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_NO_RATCHET=1",
		"INTEROP_APP_DATA=noratchet-live",
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
	hashLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("wait hash: %v", err)
	}
	pyHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(pyHash) != 16 {
		t.Fatalf("bad python destination hash: %q err %v", hashLine, err)
	}

	hexLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("wait ANNOUNCE_HEX: %v", err)
	}
	hexLine = strings.TrimSpace(hexLine)
	const hexPrefix = "ANNOUNCE_HEX "
	if !strings.HasPrefix(hexLine, hexPrefix) {
		t.Fatalf("expected ANNOUNCE_HEX, got %q", hexLine)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(hexLine, hexPrefix))
	if err != nil {
		t.Fatalf("decode announce hex: %v", err)
	}
	if len(raw) < 2 {
		t.Fatal("announce raw too short")
	}
	if (raw[0]>>5)&1 != 0 {
		t.Fatalf("expected context flag unset for no-ratchet announce, flags=%08b", raw[0])
	}

	flagLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("wait CONTEXT_FLAG: %v", err)
	}
	if strings.TrimSpace(flagLine) != "CONTEXT_FLAG 0" {
		t.Fatalf("expected CONTEXT_FLAG 0, got %q", flagLine)
	}

	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	ann, err := announce.New(id, make([]byte, 16), "liveann", nil, false, &common.ReticulumConfig{})
	if err != nil {
		t.Fatalf("announce.New: %v", err)
	}
	if err := ann.HandleAnnounce(raw); err != nil {
		t.Fatalf("HandleAnnounce on Python no-ratchet wire: %v", err)
	}

	if err := waitPath(ctx, tr, pyHash, 40*time.Second); err != nil {
		t.Fatalf("wait path after live no-ratchet announce: %v", err)
	}
	if !tr.HasPath(pyHash) {
		t.Fatal("HasPath false after Python no-ratchet announce")
	}
}

// TestLiveInteropAnnounceGoEmptyAppToPython verifies Go CreatePacket with empty
// app data is accepted by Python path learning over UDP.
func TestLiveInteropAnnounceGoEmptyAppToPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

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
	destGo.AcceptsLinks(true)

	script := pyScript(t, "wait_path.py")
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
