// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live UDP loopback cross-stack interop; set RUN_LIVE_INTEROP=1.

package interop

import (
	"bufio"
	"bytes"
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
	rlink "git.quad4.io/Networks/Reticulum-Go/pkg/link"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
	"git.quad4.io/Networks/Reticulum-Go/pkg/resource"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

const (
	interopApp    = "interop_pygo"
	interopAspect = "linksvc"

	// pyProc* bound how long the peer subprocess stays up (exec.CommandContext).
	// Multipart resources can run until link SendResource's internal deadline (10m).
	pyProcShortTimeout  = 2 * time.Minute
	pyProcMediumTimeout = 8 * time.Minute
	pyProcLongTimeout   = 22 * time.Minute
)

func interopLink(t *testing.T, v any) *rlink.Link {
	t.Helper()
	lnk, ok := v.(*rlink.Link)
	if !ok {
		t.Fatalf("expected *link.Link, got %T", v)
	}
	return lnk
}

func liveOrSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_LIVE_INTEROP") != "1" {
		t.Skip("set RUN_LIVE_INTEROP=1 to run live Python-Go interop")
	}
}

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

func scriptDir(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Dir(testFile)
}

func pyScript(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(scriptDir(t), "py", name)
}

func setupGoUDPPeer(t *testing.T, pyListen, pyForward int) (*transport.Transport, *interfaces.UDPInterface, func()) {
	t.Helper()
	cfg := &common.ReticulumConfig{}
	tr := transport.NewTransport(cfg)
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
	cleanup := func() { tr.Close() }
	return tr, iface, cleanup
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

func waitPath(ctx context.Context, tr *transport.Transport, destHash []byte, total time.Duration) error {
	deadline := time.Now().Add(total)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if tr.HasPath(destHash) {
			return nil
		}
		_ = tr.RequestPath(destHash, "interop_udp", nil, false)
		time.Sleep(80 * time.Millisecond)
	}
	return context.DeadlineExceeded
}

// Verifies the peer learns a path after Go announces.
func TestLiveInteropPythonSeesGoAnnouncePath(t *testing.T) {
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

// Verifies Go learns a path after the peer announces.
func TestLiveInteropGoSeesPythonAnnouncePath(t *testing.T) {
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
		t.Fatalf("go never got path to python destination: %v", err)
	}
}

// Establishes a link from Go (initiator) to the peer (responder) and echoes a packet.
func TestLiveInteropGoLinkPacketEchoPython(t *testing.T) {
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
		"INTEROP_LINK_MODE=echo",
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
		t.Fatalf("recall python identity: %v", err)
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
	lnk.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		replyCh <- append([]byte(nil), data...)
	})

	if err := lnk.Establish(); err != nil {
		t.Fatalf("establish: %v", err)
	}
	select {
	case <-established:
	case <-time.After(45 * time.Second):
		t.Fatal("link establish timeout")
	}
	lnk.Start()

	payload := []byte("interop-ping")
	if err := lnk.SendPacket(payload); err != nil {
		t.Fatalf("send packet: %v", err)
	}
	select {
	case got := <-replyCh:
		if string(got) != string(payload) {
			t.Fatalf("echo mismatch: %q != %q", got, payload)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("no echo reply")
	}
}

// Sends a small in-memory resource from Go to the peer over a link.
func TestLiveInteropGoResourceToPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
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
		"INTEROP_LINK_MODE=resource",
		"INTEROP_RESOURCE_EXPECT=interop-resource-payload",
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

	res, err := resource.New([]byte("interop-resource-payload"), false)
	if err != nil {
		t.Fatalf("resource: %v", err)
	}
	if err := lnk.SendResource(res); err != nil {
		t.Fatalf("send resource: %v", err)
	}

	line, err = readLineTimeout(ctx, br, 90*time.Second)
	if err != nil {
		t.Fatalf("wait RESOURCE_OK: %v", err)
	}
	if strings.TrimSpace(line) != "RESOURCE_OK" {
		t.Fatalf("expected RESOURCE_OK, got %q", line)
	}
}

// Sends two small resources on one established link (peer prints RESOURCE_OK per completion).
func TestLiveInteropGoTwoResourcesToPythonSameLink(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
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
		"INTEROP_LINK_MODE=resource",
		"INTEROP_RESOURCE_EXPECT=interop-resource-payload",
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

	payload := []byte("interop-resource-payload")
	for i := 0; i < 2; i++ {
		res, err := resource.New(payload, false)
		if err != nil {
			t.Fatalf("resource: %v", err)
		}
		if err := lnk.SendResource(res); err != nil {
			t.Fatalf("send resource %d: %v", i, err)
		}
		line, err = readLineTimeout(ctx, br, 3*time.Minute)
		if err != nil {
			t.Fatalf("wait RESOURCE_OK (%d): %v", i, err)
		}
		if strings.TrimSpace(line) != "RESOURCE_OK" {
			t.Fatalf("expected RESOURCE_OK, got %q (resource %d)", line, i)
		}
	}
}

func TestLiveInteropPythonResourceToGo(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
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

	payloadCh := make(chan []byte, 1)
	established := make(chan struct{})
	destGo.SetLinkEstablishedCallback(func(v any) {
		lnk := interopLink(t, v)
		_ = lnk.SetResourceStrategy(rlink.AcceptAll)
		lnk.SetResourceConcludedCallback(func(v any) {
			if b, ok := v.([]byte); ok {
				select {
				case payloadCh <- b:
				default:
				}
			}
		})
		lnk.Start()
		close(established)
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
		"INTEROP_LINK_CLIENT_MODE=resource_send",
		"INTEROP_RESOURCE_SEND=interop-py-to-go-payload",
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
	case <-established:
	case <-time.After(90 * time.Second):
		t.Fatal("incoming link establish timeout")
	}

	select {
	case got := <-payloadCh:
		if string(got) != "interop-py-to-go-payload" {
			t.Fatalf("payload mismatch: %q", got)
		}
	case <-time.After(90 * time.Second):
		t.Fatal("no resource payload on Go side")
	}

	line, err = readLineTimeout(ctx, br, 30*time.Second)
	if err != nil {
		t.Fatalf("wait RESOURCE_SENT_OK: %v", err)
	}
	if strings.TrimSpace(line) != "RESOURCE_SENT_OK" {
		t.Fatalf("expected RESOURCE_SENT_OK, got %q", line)
	}
}

func TestLiveInteropPythonInitiatedLinkEcho(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
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

	echoed := make(chan struct{})
	destGo.SetLinkEstablishedCallback(func(v any) {
		lnk := interopLink(t, v)
		lnk.SetPacketCallback(func(data []byte, _ *packet.Packet) {
			if err := lnk.SendPacket(data); err != nil {
				return
			}
			close(echoed)
		})
		lnk.Start()
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
		"INTEROP_LINK_CLIENT_MODE=echo",
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
	case <-echoed:
	case <-time.After(90 * time.Second):
		t.Fatal("echo path timeout")
	}

	line, err = readLineTimeout(ctx, br, 30*time.Second)
	if err != nil {
		t.Fatalf("wait ECHO_OK: %v", err)
	}
	if strings.TrimSpace(line) != "ECHO_OK" {
		t.Fatalf("expected ECHO_OK, got %q", line)
	}
}

func TestLiveInteropGoLargeResourceToPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcLongTimeout)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, iface, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	large := bytes.Repeat([]byte("L"), 5000)
	expect := string(large)

	script := pyScript(t, "link_server.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_LINK_MODE=resource",
		"INTEROP_RESOURCE_EXPECT="+expect,
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

	res, err := resource.New(large, false)
	if err != nil {
		t.Fatalf("resource: %v", err)
	}
	if err := lnk.SendResource(res); err != nil {
		t.Fatalf("send resource: %v", err)
	}

	line, err = readLineTimeout(ctx, br, 12*time.Minute)
	if err != nil {
		t.Fatalf("wait RESOURCE_OK: %v", err)
	}
	if strings.TrimSpace(line) != "RESOURCE_OK" {
		t.Fatalf("expected RESOURCE_OK, got %q", line)
	}
}

func TestLiveInteropGoCompressedResourceToPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
	defer cancel()

	compressible := bytes.Repeat([]byte("Z"), 4000)

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, iface, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	script := pyScript(t, "link_server.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_LINK_MODE=resource",
		"INTEROP_RESOURCE_EXPECT="+string(compressible),
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

	res, err := resource.New(compressible, true)
	if err != nil {
		t.Fatalf("resource: %v", err)
	}
	if err := lnk.SendResource(res); err != nil {
		t.Fatalf("send resource: %v", err)
	}

	line, err = readLineTimeout(ctx, br, 6*time.Minute)
	if err != nil {
		t.Fatalf("wait RESOURCE_OK: %v", err)
	}
	if strings.TrimSpace(line) != "RESOURCE_OK" {
		t.Fatalf("expected RESOURCE_OK, got %q", line)
	}
}

func TestLiveInteropPythonCompressedResourceToGo(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
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

	payloadCh := make(chan []byte, 1)
	established := make(chan struct{})
	destGo.SetLinkEstablishedCallback(func(v any) {
		lnk := interopLink(t, v)
		_ = lnk.SetResourceStrategy(rlink.AcceptAll)
		lnk.SetResourceConcludedCallback(func(v any) {
			if b, ok := v.([]byte); ok {
				select {
				case payloadCh <- b:
				default:
				}
			}
		})
		lnk.Start()
		close(established)
	})

	if err := destGo.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	expect := bytes.Repeat([]byte("C"), 3500)
	script := pyScript(t, "link_client.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_GO_DEST_HASH="+hex.EncodeToString(destGo.GetHash()),
		"INTEROP_LINK_CLIENT_MODE=resource_send",
		"INTEROP_RESOURCE_SEND="+string(expect),
		"INTEROP_RESOURCE_COMPRESS=1",
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
	case <-established:
	case <-time.After(90 * time.Second):
		t.Fatal("incoming link establish timeout")
	}

	select {
	case got := <-payloadCh:
		if !bytes.Equal(got, expect) {
			t.Fatalf("payload mismatch len %d vs %d", len(got), len(expect))
		}
	case <-time.After(6 * time.Minute):
		t.Fatal("no resource payload on Go side")
	}

	line, err = readLineTimeout(ctx, br, 2*time.Minute)
	if err != nil {
		t.Fatalf("wait RESOURCE_SENT_OK: %v", err)
	}
	if strings.TrimSpace(line) != "RESOURCE_SENT_OK" {
		t.Fatalf("expected RESOURCE_SENT_OK, got %q", line)
	}
}

func TestLiveInteropGoRejectIncomingResource(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
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

	destGo.SetLinkEstablishedCallback(func(v any) {
		lnk := interopLink(t, v)
		_ = lnk.SetResourceStrategy(rlink.AcceptNone)
		lnk.Start()
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
		"INTEROP_LINK_CLIENT_MODE=resource_send",
		"INTEROP_RESOURCE_SEND=reject-me",
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

	line, err = readLineTimeout(ctx, br, 90*time.Second)
	if err != nil {
		t.Fatalf("wait RESOURCE_REJECTED: %v", err)
	}
	if strings.TrimSpace(line) != "RESOURCE_REJECTED" {
		t.Fatalf("expected RESOURCE_REJECTED, got %q", line)
	}
}

func TestLiveInteropPythonLinkRequest(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
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

	const reqPath = "interop_req_path"
	if err := destGo.RegisterRequestHandler(reqPath, func(_ string, _ []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) []byte {
		return []byte("PONG_FROM_GO")
	}, destination.AllowAll, nil); err != nil {
		t.Fatalf("RegisterRequestHandler: %v", err)
	}

	destGo.SetLinkEstablishedCallback(func(v any) {
		lnk := interopLink(t, v)
		lnk.Start()
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
		"INTEROP_LINK_CLIENT_MODE=request",
		"INTEROP_REQUEST_PATH="+reqPath,
		"INTEROP_REQUEST_PAYLOAD=ping",
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

	line, err = readLineTimeout(ctx, br, 90*time.Second)
	if err != nil {
		t.Fatalf("wait REQUEST_OK: %v", err)
	}
	if strings.TrimSpace(line) != "REQUEST_OK" {
		t.Fatalf("expected REQUEST_OK, got %q", line)
	}
}

func TestLiveInteropGoFileResourceToPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
	defer cancel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "blob.bin")
	payload := bytes.Repeat([]byte("F"), 2048)
	if err := os.WriteFile(filePath, payload, 0644); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, iface, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	script := pyScript(t, "link_server.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_LINK_MODE=resource",
		"INTEROP_RESOURCE_EXPECT="+string(payload),
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

	res, err := resource.New(f, false)
	if err != nil {
		t.Fatalf("resource: %v", err)
	}
	if err := lnk.SendResource(res); err != nil {
		t.Fatalf("send resource: %v", err)
	}

	line, err = readLineTimeout(ctx, br, 6*time.Minute)
	if err != nil {
		t.Fatalf("wait RESOURCE_OK: %v", err)
	}
	if strings.TrimSpace(line) != "RESOURCE_OK" {
		t.Fatalf("expected RESOURCE_OK, got %q", line)
	}
}
