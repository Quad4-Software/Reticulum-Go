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
	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

func TestGoGoGroupUDP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	trA, _, cleanupA := setupGoUDPPeer(t, portA, portB)
	defer cleanupA()
	trB, _, cleanupB := setupGoUDPPeer(t, portB, portA)
	defer cleanupB()

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	key, err := cryptography.GenerateTokenKey()
	if err != nil {
		t.Fatal(err)
	}

	destA, err := destination.New(id, destination.In, destination.Group, interopApp, trA, "groupsvc")
	if err != nil {
		t.Fatal(err)
	}
	if err := destA.LoadPrivateKey(key); err != nil {
		t.Fatal(err)
	}
	var gotMu sync.Mutex
	var got []byte
	destA.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		gotMu.Lock()
		got = append([]byte(nil), data...)
		gotMu.Unlock()
	})

	outB, err := destination.New(id, destination.Out, destination.Group, interopApp, trB, "groupsvc")
	if err != nil {
		t.Fatal(err)
	}
	if err := outB.LoadPrivateKey(key); err != nil {
		t.Fatal(err)
	}

	plain := []byte("gogo-group")
	ct, err := outB.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	pkt := packet.NewPacket(
		packet.DestinationGroup,
		ct,
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		false,
		packet.FlagUnset,
	)
	pkt.DestinationHash = append([]byte(nil), destA.GetHash()...)
	if err := pkt.Pack(); err != nil {
		t.Fatal(err)
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
	t.Fatalf("dest A did not decrypt GROUP packet, got %q", got)
}

func TestLiveInteropGroupPythonListenGoSend(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	idBytes, err := id.GetPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	key, err := cryptography.GenerateTokenKey()
	if err != nil {
		t.Fatal(err)
	}

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	script := pyScript(t, "group_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_MODE=listen",
		"INTEROP_IDENTITY_HEX="+hex.EncodeToString(idBytes),
		"INTEROP_GROUP_KEY_HEX="+hex.EncodeToString(key),
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

	outDest, err := destination.New(id, destination.Out, destination.Group, interopApp, tr, "groupsvc")
	if err != nil {
		t.Fatal(err)
	}
	if err := outDest.LoadPrivateKey(key); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(outDest.GetHash(), pyHash) {
		t.Fatalf("dest hash mismatch go=%x py=%x", outDest.GetHash(), pyHash)
	}

	plain := []byte("hello-group")
	ct, err := outDest.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	pkt := packet.NewPacket(
		packet.DestinationGroup,
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
		t.Fatal(err)
	}
	if err := tr.SendPacket(pkt); err != nil {
		t.Fatalf("SendPacket: %v", err)
	}

	got, err := readLineTimeout(ctx, br, 20*time.Second)
	if err != nil {
		t.Fatalf("wait DECRYPTED: %v", err)
	}
	if strings.TrimSpace(got) != "DECRYPTED hello-group" {
		t.Fatalf("got %q", got)
	}
}

func TestLiveInteropGroupGoListenPythonSend(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	idBytes, err := id.GetPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	key, err := cryptography.GenerateTokenKey()
	if err != nil {
		t.Fatal(err)
	}

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	destGo, err := destination.New(id, destination.In, destination.Group, interopApp, tr, "groupsvc")
	if err != nil {
		t.Fatal(err)
	}
	if err := destGo.LoadPrivateKey(key); err != nil {
		t.Fatal(err)
	}
	var gotMu sync.Mutex
	var got []byte
	destGo.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		gotMu.Lock()
		got = append([]byte(nil), data...)
		gotMu.Unlock()
	})

	script := pyScript(t, "group_peer.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_MODE=send",
		"INTEROP_IDENTITY_HEX="+hex.EncodeToString(idBytes),
		"INTEROP_GROUP_KEY_HEX="+hex.EncodeToString(key),
		"INTEROP_PLAINTEXT=py-group-ok",
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
	if line, err := readLineTimeout(ctx, br, 20*time.Second); err != nil || strings.TrimSpace(line) != "SENT" {
		t.Fatalf("wait SENT: %q err %v", line, err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		gotMu.Lock()
		ok := bytes.Equal(got, []byte("py-group-ok"))
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
	t.Fatalf("Go dest did not decrypt Python GROUP ciphertext, got %q", got)
}
