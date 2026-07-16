// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live channel and buffer stream interop with Python RNS.
// Set RUN_LIVE_INTEROP=1.

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

	"quad4/reticulum-go/pkg/buffer"
	"quad4/reticulum-go/pkg/channel"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	rlink "quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/packet"
)

type echoMsg struct {
	data []byte
}

func (m *echoMsg) Pack() ([]byte, error) { return m.data, nil }
func (m *echoMsg) Unpack(raw []byte) error {
	m.data = append([]byte(nil), raw...)
	return nil
}
func (m *echoMsg) GetType() uint16 { return 1 }

// Go initiates a link and exchanges a typed channel message with Python.
func TestLiveInteropGoChannelEchoPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, iface, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	script := pyScript(t, "link_server.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_LINK_MODE=channel",
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
	if err := waitPath(ctx, tr, pyHash, 40*time.Second); err != nil {
		t.Fatalf("path to python: %v", err)
	}

	srvID, err := identity.Recall(pyHash)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	destOut, err := destination.FromHash(pyHash, srvID, destination.Single, tr)
	if err != nil {
		t.Fatalf("from hash: %v", err)
	}

	established := make(chan struct{})
	replyCh := make(chan []byte, 1)
	lnk := rlink.NewLink(destOut, tr, iface, func(_ *rlink.Link) {
		close(established)
	}, nil)
	defer lnk.Teardown()
	lnk.SetPacketCallback(func(_ []byte, _ *packet.Packet) {})

	if err := lnk.Establish(); err != nil {
		t.Fatalf("establish: %v", err)
	}
	select {
	case <-established:
	case <-time.After(45 * time.Second):
		t.Fatal("link establish timeout")
	}
	lnk.Start()

	ch := lnk.GetChannel()
	if err := ch.RegisterMessageType(1, func() channel.MessageBase { return &echoMsg{} }); err != nil {
		t.Fatal(err)
	}
	ch.AddMessageHandler(func(m channel.MessageBase) bool {
		em, ok := m.(*echoMsg)
		if !ok {
			return false
		}
		replyCh <- append([]byte(nil), em.data...)
		return true
	})

	payload := []byte("interop-channel-ping")
	if err := ch.Send(&echoMsg{data: payload}); err != nil {
		t.Fatalf("channel send: %v", err)
	}
	select {
	case got := <-replyCh:
		if string(got) != string(payload) {
			t.Fatalf("channel echo mismatch: %q != %q", got, payload)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("no channel echo reply")
	}
}

// Go sends a buffer stream to Python over an established link.
func TestLiveInteropGoBufferToPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, iface, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	script := pyScript(t, "link_server.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_LINK_MODE=buffer",
		"INTEROP_BUFFER_EXPECT=interop-buffer-payload",
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
	if err := waitPath(ctx, tr, pyHash, 40*time.Second); err != nil {
		t.Fatalf("path to python: %v", err)
	}

	srvID, err := identity.Recall(pyHash)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	destOut, err := destination.FromHash(pyHash, srvID, destination.Single, tr)
	if err != nil {
		t.Fatalf("from hash: %v", err)
	}

	established := make(chan struct{})
	lnk := rlink.NewLink(destOut, tr, iface, func(_ *rlink.Link) {
		close(established)
	}, nil)
	defer lnk.Teardown()
	lnk.SetPacketCallback(func(_ []byte, _ *packet.Packet) {})

	if err := lnk.Establish(); err != nil {
		t.Fatalf("establish: %v", err)
	}
	select {
	case <-established:
	case <-time.After(45 * time.Second):
		t.Fatal("link establish timeout")
	}
	lnk.Start()

	ch := lnk.GetChannel()
	payload := []byte("interop-buffer-payload")
	raw := buffer.NewRawChannelWriter(1, ch)
	if _, err := raw.Write(payload); err != nil {
		t.Fatalf("buffer write: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("buffer close: %v", err)
	}

	line, err = readLineTimeout(ctx, br, 30*time.Second)
	if err != nil {
		t.Fatalf("wait BUFFER_OK: %v", err)
	}
	if strings.TrimSpace(line) != "BUFFER_OK" {
		t.Fatalf("expected BUFFER_OK, got %q", line)
	}
}
