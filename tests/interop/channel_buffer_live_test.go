// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live channel and buffer stream interop with Python RNS.
// Set RUN_LIVE_INTEROP=1.

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
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/buffer"
	"github.com/Quad4-Software/Reticulum-Go/pkg/channel"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	rlink "github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
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

// Go sends a compressible buffer stream (> CompressThreshold) so the bz2
// wire path is exercised against Python RNS.Buffer.
func TestLiveInteropGoCompressedBufferToPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, iface, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	payload := bytes.Repeat([]byte("interop-buffer-compressed-payload-"), 8)
	if len(payload) <= buffer.CompressThreshold {
		t.Fatalf("payload %d must exceed CompressThreshold %d", len(payload), buffer.CompressThreshold)
	}

	script := pyScript(t, "link_server.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_LINK_MODE=buffer",
		"INTEROP_BUFFER_EXPECT="+string(payload),
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
	raw := buffer.NewRawChannelWriter(1, ch)
	n, err := raw.Write(payload)
	if err != nil {
		t.Fatalf("buffer write: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("buffer write processed %d, want %d (compression should cover full chunk)", n, len(payload))
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

// Python streams a buffer payload to Go over a Python-initiated link.
func TestLiveInteropPythonBufferToGo(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	expect := []byte("interop-buffer-payload")
	got := make(chan []byte, 1)

	idGo, err := identity.New()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	destGo, err := destination.New(idGo, destination.In, destination.Single, interopApp, tr, interopAspect)
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	destGo.AcceptsLinks(true)
	destGo.SetLinkEstablishedCallback(func(v any) {
		lnk := interopLink(t, v)
		lnk.Start()
		ready := make(chan int, 1)
		reader := buffer.CreateReader(1, lnk.GetChannel(), func(n int) {
			select {
			case ready <- n:
			default:
			}
		})
		go func() {
			buf := make([]byte, 0, len(expect))
			tmp := make([]byte, 256)
			for len(buf) < len(expect) {
				select {
				case <-ready:
				case <-time.After(30 * time.Second):
					return
				}
				for {
					n, err := reader.Read(tmp)
					buf = append(buf, tmp[:n]...)
					if err != nil || n == 0 {
						break
					}
				}
			}
			got <- buf[:len(expect)]
		}()
	})
	if err := destGo.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	script := pyScript(t, "link_client.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_GO_DEST_HASH="+hex.EncodeToString(destGo.GetHash()),
		"INTEROP_LINK_CLIENT_MODE=buffer_send",
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

	select {
	case data := <-got:
		if !bytes.Equal(data, expect) {
			t.Fatalf("buffer payload = %q, want %q", data, expect)
		}
	case <-ctx.Done():
		t.Fatalf("buffer read: %v", ctx.Err())
	}
}
