// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

// TestLiveInteropRatchetPythonAnnounceGoEncrypt sends identity-encrypted DATA
// to a Python destination that enabled ratchets. Go must remember the announced
// ratchet and encrypt to it so Python decrypt succeeds under enforce_ratchets.
func TestLiveInteropRatchetPythonAnnounceGoEncrypt(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	script := pyScript(t, "ratchet_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_MODE=announce",
		"INTEROP_APP_DATA=ratchet-py-announce",
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
	if line, err := readLineTimeout(ctx, br, 25*time.Second); err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("wait READY: %q err %v", line, err)
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
	if !strings.HasPrefix(strings.TrimSpace(hexLine), "ANNOUNCE_HEX ") {
		t.Fatalf("expected ANNOUNCE_HEX, got %q", hexLine)
	}
	flagLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("wait CONTEXT_FLAG: %v", err)
	}
	if strings.TrimSpace(flagLine) != "CONTEXT_FLAG 1" {
		t.Fatalf("expected CONTEXT_FLAG 1, got %q", flagLine)
	}

	if err := waitPath(ctx, tr, pyHash, 40*time.Second); err != nil {
		t.Fatalf("wait path: %v", err)
	}
	ratchet := identity.GetRatchet(pyHash)
	if len(ratchet) != 32 {
		t.Fatalf("Go did not remember announced ratchet, len=%d", len(ratchet))
	}

	remote, err := identity.Recall(pyHash)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	outDest, err := destination.FromHash(pyHash, remote, destination.Single, tr)
	if err != nil {
		t.Fatalf("FromHash: %v", err)
	}
	plain := []byte("hello-ratchet")
	ct, err := outDest.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(outDest.LatestRatchetID()) == 0 {
		t.Fatal("encrypt did not record ratchet id")
	}
	pkt := packet.NewPacket(
		packet.DestinationSingle,
		ct,
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		false,
		packet.FlagUnset,
	)
	pkt.DestinationHash = append([]byte(nil), pyHash...)
	if err := pkt.Pack(); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if err := tr.SendPacket(pkt); err != nil {
		t.Fatalf("SendPacket: %v", err)
	}

	got, err := readLineTimeout(ctx, br, 20*time.Second)
	if err != nil {
		t.Fatalf("wait DECRYPTED: %v", err)
	}
	if strings.TrimSpace(got) != "DECRYPTED hello-ratchet" {
		t.Fatalf("got %q", got)
	}
}

// TestLiveInteropRatchetGoAnnouncePythonEncrypt is the reverse: Go enables
// ratchets and announces, Python encrypts to the remembered ratchet, Go decrypts.
func TestLiveInteropRatchetGoAnnouncePythonEncrypt(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	idGo, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destGo, err := destination.New(idGo, destination.In, destination.Single, interopApp, tr, interopAspect)
	if err != nil {
		t.Fatal(err)
	}
	if !destGo.EnableRatchetsInMemory() {
		t.Fatal("EnableRatchetsInMemory")
	}
	var gotMu sync.Mutex
	var got []byte
	destGo.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		gotMu.Lock()
		got = append([]byte(nil), data...)
		gotMu.Unlock()
	})

	script := pyScript(t, "ratchet_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_MODE=encrypt",
		"INTEROP_GO_DEST_HASH="+hex.EncodeToString(destGo.GetHash()),
		"INTEROP_PLAINTEXT=go-ratchet-ok",
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
	if line, err := readLineTimeout(ctx, br, 25*time.Second); err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("wait READY: %q err %v", line, err)
	}
	if _, err := readLineTimeout(ctx, br, 5*time.Second); err != nil {
		t.Fatalf("wait python hash: %v", err)
	}

	if err := destGo.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	line, err := readLineTimeout(ctx, br, 45*time.Second)
	if err != nil {
		t.Fatalf("wait SENT: %v", err)
	}
	if strings.TrimSpace(line) != "SENT" {
		t.Fatalf("expected SENT, got %q", line)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		gotMu.Lock()
		ok := bytes.Equal(got, []byte("go-ratchet-ok"))
		gotMu.Unlock()
		if ok {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	gotMu.Lock()
	defer gotMu.Unlock()
	t.Fatalf("Go dest did not decrypt Python ratchet ciphertext, got %q", got)
}

func TestGoGoRatchetUDP(t *testing.T) {
	t.Run("in_memory", func(t *testing.T) {
		runGoGoRatchetUDP(t, "")
	})
	t.Run("file", func(t *testing.T) {
		runGoGoRatchetUDP(t, t.TempDir())
	})
}

func runGoGoRatchetUDP(t *testing.T, ratchetDir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	trA, _, cleanupA := setupGoUDPPeer(t, portA, portB)
	defer cleanupA()
	trB, _, cleanupB := setupGoUDPPeer(t, portB, portA)
	defer cleanupB()

	idA, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destA, err := destination.New(idA, destination.In, destination.Single, interopApp, trA, "ratchet")
	if err != nil {
		t.Fatal(err)
	}
	if ratchetDir == "" {
		if !destA.EnableRatchetsInMemory() {
			t.Fatal("EnableRatchetsInMemory")
		}
	} else {
		path := ratchetDir + "/dest_ratchets"
		if !destA.EnableRatchets(path) {
			t.Fatal("EnableRatchets")
		}
	}
	destA.EnforceRatchets()

	var gotMu sync.Mutex
	var got []byte
	destA.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		gotMu.Lock()
		got = append([]byte(nil), data...)
		gotMu.Unlock()
	})

	if err := destA.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	hashA := destA.GetHash()
	if err := waitPath(ctx, trB, hashA, 15*time.Second); err != nil {
		t.Fatalf("wait path: %v", err)
	}
	ratchet := identity.GetRatchet(hashA)
	if len(ratchet) != 32 {
		t.Fatalf("peer B did not remember announced ratchet, len=%d", len(ratchet))
	}

	remote, err := identity.Recall(hashA)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	outDest, err := destination.FromHash(hashA, remote, destination.Single, trB)
	if err != nil {
		t.Fatalf("FromHash: %v", err)
	}
	plain := []byte("gogo-ratchet")
	ct, err := outDest.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(outDest.LatestRatchetID()) == 0 {
		t.Fatal("encrypt did not record ratchet id")
	}
	pkt := packet.NewPacket(
		packet.DestinationSingle,
		ct,
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		false,
		packet.FlagUnset,
	)
	pkt.DestinationHash = append([]byte(nil), hashA...)
	if err := pkt.Pack(); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if err := trB.SendPacket(pkt); err != nil {
		t.Fatalf("SendPacket: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		gotMu.Lock()
		ok := bytes.Equal(got, plain)
		gotMu.Unlock()
		if ok {
			if ratchetDir != "" {
				if _, err := os.Stat(ratchetDir + "/dest_ratchets"); err != nil {
					t.Fatalf("expected dest ratchet file: %v", err)
				}
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	gotMu.Lock()
	defer gotMu.Unlock()
	t.Fatalf("dest A did not decrypt Go ratchet ciphertext, got %q", got)
}
