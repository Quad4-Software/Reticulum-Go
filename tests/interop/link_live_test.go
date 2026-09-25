// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live UDP loopback cross-stack interop. Set RUN_LIVE_INTEROP=1.

package interop

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	rlink "github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/Reticulum-Go/tests/interop/harness"
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
	return harness.PythonExe()
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
	return harness.ReadLineTimeout(ctx, br, d)
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
	for i := range 2 {
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

func TestLiveInteropPythonLargeResourceToGo(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcLongTimeout)
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

	large := strings.Repeat("L", 18000) + "\nPY_LARGE_TO_GO\n" + strings.Repeat("M", 18000)
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
		"INTEROP_RESOURCE_SEND="+large,
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
		if string(got) != large {
			t.Fatalf("large payload mismatch: got=%d want=%d", len(got), len(large))
		}
		if !bytes.Contains(got, []byte("PY_LARGE_TO_GO")) {
			t.Fatal("missing PY_LARGE_TO_GO marker")
		}
	case <-time.After(12 * time.Minute):
		t.Fatal("no large resource payload on Go side")
	}

	line, err = readLineTimeout(ctx, br, 2*time.Minute)
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

// TestLiveInteropGoBinaryPacketBurstEchoPython sends several binary payloads
// (including HDLC special bytes) to a Python echo peer and checks byte-exact replies.
func TestLiveInteropGoBinaryPacketBurstEchoPython(t *testing.T) {
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
	replyCh := make(chan []byte, 8)
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

	payloads := [][]byte{
		{0x00, 0x7E, 0x7D, 0xFF},
		bytes.Repeat([]byte{0xA5}, 64),
		[]byte("interop-bin\x00burst"),
		{0x01, 0x02, 0x03, 0x04, 0x05},
	}
	for i, payload := range payloads {
		if err := lnk.SendPacket(payload); err != nil {
			t.Fatalf("send packet %d: %v", i, err)
		}
		select {
		case got := <-replyCh:
			if !bytes.Equal(got, payload) {
				t.Fatalf("echo %d mismatch: got %x want %x", i, got, payload)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("no echo reply for packet %d", i)
		}
	}
}

// TestLiveInteropGoLinkRequestToPython has Go open a link to Python and issue
// a destination request that Python answers with a fixed reply.
func TestLiveInteropGoLinkRequestToPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, iface, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	const (
		reqPath    = "interop_go_req"
		reqPayload = "ping-from-go"
		reqReply   = "PONG_FROM_PY"
	)

	script := pyScript(t, "link_server.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_LINK_MODE=echo",
		"INTEROP_REQUEST_PATH="+reqPath,
		"INTEROP_REQUEST_PAYLOAD="+reqPayload,
		"INTEROP_REQUEST_REPLY="+reqReply,
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
	lnk := rlink.NewLink(destOut, tr, iface, func(_ *rlink.Link) {
		close(established)
	}, nil)
	defer lnk.Teardown()

	if err := lnk.Establish(); err != nil {
		t.Fatalf("establish: %v", err)
	}
	select {
	case <-established:
	case <-time.After(45 * time.Second):
		t.Fatal("link establish timeout")
	}
	lnk.Start()

	done := make(chan []byte, 1)
	fail := make(chan struct{}, 1)
	receipt, err := lnk.Request(reqPath, []byte(reqPayload), 20*time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	receipt.SetResponseCallback(func(r *rlink.RequestReceipt) {
		done <- append([]byte(nil), r.GetResponse()...)
	})
	receipt.SetFailedCallback(func(_ *rlink.RequestReceipt) {
		select {
		case fail <- struct{}{}:
		default:
		}
	})

	select {
	case got := <-done:
		if string(got) != reqReply {
			t.Fatalf("request reply mismatch: %q != %q", got, reqReply)
		}
	case <-fail:
		t.Fatal("request failed")
	case <-time.After(25 * time.Second):
		t.Fatal("request response timeout")
	}
}

// goLinkToPythonServer starts link_server.py, waits for the Go peer to learn
// its path, and returns an established Go link. The returned reader is left
// positioned after the READY and destination-hash lines so tests can read any
// extra lines the server emits (for example REPLY_SHA256).
func goLinkToPythonServer(t *testing.T, ctx context.Context, serverEnv []string) (*rlink.Link, *bufio.Reader, func()) {
	t.Helper()
	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, iface, trCleanup := setupGoUDPPeer(t, pyListen, pyForward)

	fail := func(format string, args ...any) {
		trCleanup()
		t.Fatalf(format, args...)
	}

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "link_server.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_LINK_MODE=echo",
	)
	cmd.Env = append(cmd.Env, serverEnv...)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		fail("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		fail("start python: %v", err)
	}
	killOnFatal := func(format string, args ...any) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		fail(format, args...)
	}

	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 25*time.Second)
	if err != nil {
		killOnFatal("wait READY: %v", err)
	}
	if strings.TrimSpace(line) != "READY" {
		killOnFatal("expected READY, got %q", line)
	}
	hashLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		killOnFatal("wait hash: %v", err)
	}
	pyHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(pyHash) != 16 {
		killOnFatal("bad python destination hash: %q err %v", hashLine, err)
	}
	if err := waitPath(ctx, tr, pyHash, 40*time.Second); err != nil {
		killOnFatal("path to python: %v", err)
	}

	srvID, err := identity.Recall(pyHash)
	if err != nil {
		killOnFatal("recall python identity: %v", err)
	}
	destOut, err := destination.FromHash(pyHash, srvID, destination.Single, tr)
	if err != nil {
		killOnFatal("from hash: %v", err)
	}

	established := make(chan struct{})
	lnk := rlink.NewLink(destOut, tr, iface, func(_ *rlink.Link) {
		close(established)
	}, nil)

	if err := lnk.Establish(); err != nil {
		killOnFatal("establish: %v", err)
	}
	select {
	case <-established:
	case <-time.After(45 * time.Second):
		killOnFatal("link establish timeout")
	}
	lnk.Start()

	cleanup := func() {
		lnk.Teardown()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		trCleanup()
	}
	return lnk, br, cleanup
}

// readServerSHALine reads a "KEY <hex>" line emitted by link_server.py or
// link_client.py and returns the hex digest portion.
func readServerSHALine(t *testing.T, ctx context.Context, br *bufio.Reader, prefix string) string {
	t.Helper()
	line, err := readLineTimeout(ctx, br, 10*time.Second)
	if err != nil {
		t.Fatalf("wait %s line: %v", prefix, err)
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, prefix+" ") {
		t.Fatalf("expected %q line, got %q", prefix, line)
	}
	return strings.TrimSpace(strings.TrimPrefix(line, prefix))
}

// Python answers a Go request with a random reply larger than the link MDU,
// which forces a resource transfer on the response path. This exercises the
// request receipt states changed in the lifecycle fix: the receipt must move
// through receiving to ready without its own deadline firing mid-transfer.
func TestLiveInteropGoLinkRequestLargeResponsePython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
	defer cancel()

	const replySize = 600 * 1024
	lnk, br, cleanup := goLinkToPythonServer(t, ctx, []string{
		"INTEROP_REQUEST_PATH=interop_go_req_big",
		"INTEROP_REQUEST_PAYLOAD=ping-big",
		"INTEROP_REQUEST_REPLY_SIZE=" + strconv.Itoa(replySize),
	})
	defer cleanup()

	replySHA := readServerSHALine(t, ctx, br, "REPLY_SHA256")

	var progressSeen atomic.Int64
	done := make(chan []byte, 1)
	fail := make(chan struct{}, 1)

	receipt, err := lnk.Request("interop_go_req_big", []byte("ping-big"), 60*time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	receipt.SetProgressCallback(func(r *rlink.RequestReceipt) {
		if n, _ := r.Progress(); n > progressSeen.Load() {
			progressSeen.Store(n)
		}
	})
	receipt.SetResponseCallback(func(r *rlink.RequestReceipt) {
		done <- append([]byte(nil), r.GetResponse()...)
	})
	receipt.SetFailedCallback(func(_ *rlink.RequestReceipt) {
		select {
		case fail <- struct{}{}:
		default:
		}
	})

	select {
	case got := <-done:
		if int64(len(got)) != replySize {
			t.Fatalf("response size %d want %d", len(got), replySize)
		}
		sum := sha256.Sum256(got)
		if hex.EncodeToString(sum[:]) != replySHA {
			t.Fatal("response sha256 mismatch")
		}
		if st := receipt.GetStatus(); st != rlink.StatusActive {
			t.Fatalf("receipt status %#x want active", st)
		}
		if progressSeen.Load() == 0 {
			t.Fatal("no progress observed during resource response")
		}
	case <-fail:
		t.Fatal("request failed during resource response")
	case <-ctx.Done():
		t.Fatal("request response timeout")
	}
}

// Same as above but the reply exceeds Resource.MAX_EFFICIENT_SIZE so Python
// splits it into sequential segment transfers. Verifies the request binding
// survives each segment handoff.
func TestLiveInteropGoLinkRequestSplitResponsePython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
	defer cancel()

	replySize := int64(resource.MaxEfficientSize) + 400*1024
	lnk, br, cleanup := goLinkToPythonServer(t, ctx, []string{
		"INTEROP_REQUEST_PATH=interop_go_req_split",
		"INTEROP_REQUEST_PAYLOAD=ping-split",
		"INTEROP_REQUEST_REPLY_SIZE=" + strconv.FormatInt(replySize, 10),
	})
	defer cleanup()

	replySHA := readServerSHALine(t, ctx, br, "REPLY_SHA256")

	done := make(chan []byte, 1)
	fail := make(chan struct{}, 1)

	receipt, err := lnk.Request("interop_go_req_split", []byte("ping-split"), 120*time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	receipt.SetResponseCallback(func(r *rlink.RequestReceipt) {
		done <- append([]byte(nil), r.GetResponse()...)
	})
	receipt.SetFailedCallback(func(_ *rlink.RequestReceipt) {
		select {
		case fail <- struct{}{}:
		default:
		}
	})

	select {
	case got := <-done:
		if int64(len(got)) != replySize {
			t.Fatalf("response size %d want %d", len(got), replySize)
		}
		sum := sha256.Sum256(got)
		if hex.EncodeToString(sum[:]) != replySHA {
			t.Fatal("split response sha256 mismatch")
		}
	case <-fail:
		t.Fatal("request failed during split resource response")
	case <-ctx.Done():
		t.Fatal("split request response timeout")
	}
}

// Python registers the request path but never replies. Each timed-out receipt
// must be removed from pendingRequests so the link neither wedges on
// MaxPendingRequests nor reports ErrLinkRequestDuplicate on the same path.
func TestLiveInteropGoLinkRequestTimeoutRecoveryPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	lnk, _, cleanup := goLinkToPythonServer(t, ctx, []string{
		"INTEROP_REQUEST_PATH=interop_silent",
		"INTEROP_REQUEST_PAYLOAD=ping-silent",
		"INTEROP_REQUEST_NOREPLY=1",
	})
	defer cleanup()

	for i := 0; i < rlink.MaxPendingRequests+2; i++ {
		receipt, err := lnk.Request("interop_silent", []byte("ping-silent"), 1500*time.Millisecond)
		if err != nil {
			t.Fatalf("request %d rejected: %v (link status=%d)", i, err, lnk.GetStatus())
		}
		deadline := time.Now().Add(15 * time.Second)
		for receipt.GetStatus() == rlink.StatusPending && time.Now().Before(deadline) {
			time.Sleep(25 * time.Millisecond)
		}
		if st := receipt.GetStatus(); st != rlink.StatusFailed {
			t.Fatalf("request %d status %#x want failed", i, st)
		}
	}
}

// goRequestClientServer spins up an incoming Go destination that accepts
// links, registers reqPath, announces, then starts link_client.py in request
// mode against it. Returns the client stdout reader and a cleanup function.
func goRequestClientServer(t *testing.T, ctx context.Context, reqPath string, handler func(string, []byte, []byte, []byte, *identity.Identity, int64) []byte, clientEnv []string) (*bufio.Reader, func()) {
	t.Helper()
	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, trCleanup := setupGoUDPPeer(t, pyListen, pyForward)

	idGo, err := identity.New()
	if err != nil {
		trCleanup()
		t.Fatalf("identity: %v", err)
	}
	destGo, err := destination.New(idGo, destination.In, destination.Single, interopApp, tr, interopAspect)
	if err != nil {
		trCleanup()
		t.Fatalf("destination: %v", err)
	}
	destGo.AcceptsLinks(true)

	if err := destGo.RegisterRequestHandler(reqPath, handler, destination.AllowAll, nil); err != nil {
		trCleanup()
		t.Fatalf("RegisterRequestHandler: %v", err)
	}
	destGo.SetLinkEstablishedCallback(func(v any) {
		lnk := interopLink(t, v)
		lnk.Start()
	})
	if err := destGo.Announce(false, nil, nil); err != nil {
		trCleanup()
		t.Fatalf("announce: %v", err)
	}

	fail := func(format string, args ...any) {
		trCleanup()
		t.Fatalf(format, args...)
	}

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "link_client.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_GO_DEST_HASH="+hex.EncodeToString(destGo.GetHash()),
		"INTEROP_LINK_CLIENT_MODE=request",
		"INTEROP_REQUEST_PATH="+reqPath,
	)
	cmd.Env = append(cmd.Env, clientEnv...)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		fail("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		fail("start python client: %v", err)
	}
	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 30*time.Second)
	if err != nil || strings.TrimSpace(line) != "READY" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		fail("client not ready: %q %v", line, err)
	}

	cleanup := func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		trCleanup()
	}
	return br, cleanup
}

// Python requests a large response from the Go side. The Go handler replies
// with random data over the link MDU so sendResponse emits a resource. The
// Python client verifies the payload hash and prints REQUEST_OK.
func TestLiveInteropPythonLinkRequestLargeResponseGo(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
	defer cancel()

	const replySize = 600 * 1024
	reply := make([]byte, replySize)
	if _, err := rand.Read(reply); err != nil {
		t.Fatalf("reply rand: %v", err)
	}
	replySum := sha256.Sum256(reply)

	br, cleanup := goRequestClientServer(t, ctx, "interop_py_req_big",
		func(_ string, _ []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) []byte {
			return reply
		},
		[]string{
			"INTEROP_REQUEST_PAYLOAD=ping",
			"INTEROP_EXPECT_REPLY_SHA256=" + hex.EncodeToString(replySum[:]),
		})
	defer cleanup()

	deadline := time.Now().Add(240 * time.Second)
	for time.Now().Before(deadline) {
		line, err := readLineTimeout(ctx, br, 15*time.Second)
		if err != nil {
			t.Fatalf("client wait: %v", err)
		}
		switch strings.TrimSpace(line) {
		case "REQUEST_OK":
			return
		case "LINK_FAILED":
			t.Fatal("python link failed")
		}
	}
	t.Fatal("no REQUEST_OK within deadline")
}

// Python sends a request payload larger than the link MDU, which forces the
// request itself to arrive as an incoming resource on the Go side. The Go
// handler verifies the payload hash before replying.
func TestLiveInteropPythonLinkRequestResourceToGo(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
	defer cancel()

	const payloadSize = 600 * 1024
	payloadOK := make(chan struct{}, 1)
	var expectedPayloadSHA atomic.Value
	expectedPayloadSHA.Store("")

	br, cleanup := goRequestClientServer(t, ctx, "interop_py_req_resource",
		func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) []byte {
			if len(data) != payloadSize {
				t.Errorf("request payload size %d want %d", len(data), payloadSize)
				return nil
			}
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != expectedPayloadSHA.Load().(string) {
				t.Errorf("request payload sha256 mismatch")
				return nil
			}
			select {
			case payloadOK <- struct{}{}:
			default:
			}
			return []byte("PONG_FROM_GO")
		},
		[]string{
			"INTEROP_REQUEST_PAYLOAD_SIZE=" + strconv.Itoa(payloadSize),
		})
	defer cleanup()

	deadline := time.Now().Add(240 * time.Second)
	for time.Now().Before(deadline) {
		line, err := readLineTimeout(ctx, br, 15*time.Second)
		if err != nil {
			t.Fatalf("client wait: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PAYLOAD_SHA256 ") {
			expectedPayloadSHA.Store(strings.TrimSpace(strings.TrimPrefix(line, "PAYLOAD_SHA256")))
			continue
		}
		switch line {
		case "REQUEST_OK":
			select {
			case <-payloadOK:
			case <-time.After(5 * time.Second):
				t.Fatal("response ok but Go handler never verified payload")
			}
			return
		case "LINK_FAILED":
			t.Fatal("python link failed")
		}
	}
	t.Fatal("no REQUEST_OK within deadline")
}
