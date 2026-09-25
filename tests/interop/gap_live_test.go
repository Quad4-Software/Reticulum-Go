// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live UDP loopback cross-stack interop. Set RUN_LIVE_INTEROP=1.

package interop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/channel"
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	"github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

// Python initiates a link then tears it down; Go must observe the
// remote-initiated close through the same wire teardown packet.
func TestLiveInteropPythonTeardownSeenByGo(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, iface, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "link_teardown_server.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
	)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	if line, err := readLineTimeout(ctx, br, 20*time.Second); err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q err=%v", line, err)
	}
	destHex, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("dest hash line: %v", err)
	}
	destHash, err := hex.DecodeString(strings.TrimSpace(destHex))
	if err != nil || len(destHash) != 16 {
		t.Fatalf("bad dest hash %q", destHex)
	}

	if err := waitPath(ctx, tr, destHash, 40*time.Second); err != nil {
		t.Fatalf("path to python: %v", err)
	}
	srvID, err := identity.Recall(destHash)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	destOut, err := destination.FromHash(destHash, srvID, destination.Single, tr)
	if err != nil {
		t.Fatalf("from hash: %v", err)
	}

	closed := make(chan struct{}, 1)
	l := link.NewLink(destOut, tr, iface, nil, func(_ *link.Link) {
		select {
		case closed <- struct{}{}:
		default:
		}
	})
	if err := l.Establish(); err != nil {
		t.Fatalf("establish: %v", err)
	}

	line, err := readLineTimeout(ctx, br, 30*time.Second)
	if err != nil || strings.TrimSpace(line) != "LINK_UP" {
		t.Fatalf("expected LINK_UP, got %q err=%v", line, err)
	}

	// Signal the Python side to tear the link down remotely.
	_, _ = stdin.Write([]byte("teardown\n"))
	if line, err := readLineTimeout(ctx, br, 15*time.Second); err != nil || strings.TrimSpace(line) != "TORN" {
		t.Fatalf("expected TORN, got %q err=%v", line, err)
	}

	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatalf("go link closed callback never fired; status=%v", l.GetStatus())
	}
	if st := l.GetStatus(); st == link.StatusActive || st == link.StatusPending || st == link.StatusHandshake {
		t.Fatalf("link still active after remote teardown: %v", st)
	}
}

// Two independent Go links to one Python destination must coexist with
// distinct link IDs, and tearing one down must not kill the other.
func TestLiveInteropGoTwoLinksSamePythonDest(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, iface, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "link_server.py"))
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
	if line, err := readLineTimeout(ctx, br, 20*time.Second); err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q err=%v", line, err)
	}
	destHex, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("dest hash: %v", err)
	}
	destHash, err := hex.DecodeString(strings.TrimSpace(destHex))
	if err != nil || len(destHash) != 16 {
		t.Fatalf("bad dest hash %q", destHex)
	}

	if err := waitPath(ctx, tr, destHash, 40*time.Second); err != nil {
		t.Fatalf("path to python: %v", err)
	}
	srvID, err := identity.Recall(destHash)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	destOut, err := destination.FromHash(destHash, srvID, destination.Single, tr)
	if err != nil {
		t.Fatalf("from hash: %v", err)
	}

	mkLink := func(name string) *link.Link {
		l := link.NewLink(destOut, tr, iface, nil, nil)
		if err := l.Establish(); err != nil {
			t.Fatalf("%s establish: %v", name, err)
		}
		deadline := time.Now().Add(15 * time.Second)
		for l.GetStatus() != link.StatusActive && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if l.GetStatus() != link.StatusActive {
			t.Fatalf("%s never became active: %v", name, l.GetStatus())
		}
		return l
	}
	l1 := mkLink("l1")
	l2 := mkLink("l2")
	if string(l1.GetLinkID()) == string(l2.GetLinkID()) {
		t.Fatal("link IDs must differ")
	}
	l1.Teardown()
	time.Sleep(200 * time.Millisecond)
	if st := l2.GetStatus(); st != link.StatusActive {
		t.Fatalf("teardown of l1 killed l2: %v", st)
	}
	l2.Teardown()
}

// A announces through two chained Go relays; the receiving Python node
// must see the announce at the propagated hop count.
func TestLiveInteropMultiHopAnnounceProgression(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// A -> R1 -> R2 -> C
	al, r1a := freeUDPPort(t), freeUDPPort(t)
	r1r2, r2a := freeUDPPort(t), freeUDPPort(t)
	r2b, cl := freeUDPPort(t), freeUDPPort(t)

	r1, r1Cleanup := setupGoUDPRelay(t, r1a, al, r1r2, r2a)
	defer r1Cleanup()
	r2, r2Cleanup := setupGoUDPRelay(t, r2a, r1r2, r2b, cl)
	defer r2Cleanup()

	pyACmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "announce_peer.py"))
	pyACmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(al),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(r1a),
	)
	pyACmd.Stderr = os.Stderr
	pyAOut, err := pyACmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pyA pipe: %v", err)
	}
	if err := pyACmd.Start(); err != nil {
		t.Fatalf("start pyA: %v", err)
	}
	defer func() {
		_ = pyACmd.Process.Kill()
		_ = pyACmd.Wait()
	}()

	pyABr := bufio.NewReader(pyAOut)
	if line, err := readLineTimeout(ctx, pyABr, 25*time.Second); err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("pyA READY: %q %v", line, err)
	}
	hashLine, err := readLineTimeout(ctx, pyABr, 5*time.Second)
	if err != nil {
		t.Fatalf("pyA hash: %v", err)
	}
	aHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(aHash) != 16 {
		t.Fatalf("bad hash %q", hashLine)
	}

	pyCCmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "check_path_hops.py"))
	pyCCmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(cl),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(r2b),
		"INTEROP_TARGET_HASH="+hex.EncodeToString(aHash),
	)
	pyCCmd.Stderr = os.Stderr
	pyCOut, err := pyCCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pyC pipe: %v", err)
	}
	if err := pyCCmd.Start(); err != nil {
		t.Fatalf("start pyC: %v", err)
	}
	defer func() {
		_ = pyCCmd.Process.Kill()
		_ = pyCCmd.Wait()
	}()

	pyCBr := bufio.NewReader(pyCOut)
	if line, err := readLineTimeout(ctx, pyCBr, 25*time.Second); err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("pyC READY: %q %v", line, err)
	}
	line, err := readLineTimeout(ctx, pyCBr, 60*time.Second)
	if err != nil {
		t.Fatalf("pyC PATH_HOPS: %v", err)
	}
	got := strings.TrimSpace(line)
	if !strings.HasPrefix(got, "PATH_HOPS ") {
		t.Fatalf("expected PATH_HOPS line, got %q", got)
	}
	hops, err := strconv.Atoi(strings.TrimPrefix(got, "PATH_HOPS "))
	if err != nil {
		t.Fatalf("bad hops %q", got)
	}
	// A emits 0; R1 forwards wire=1; R2 forwards wire=2; C parses +1
	// into its path table entry, which Python stores as hops taken.
	if hops != 3 {
		t.Fatalf("expected 3 hops through two relays, got %d", hops)
	}

	// R1 and R2 must have learned the path at the right cost too.
	if !r1.HasPath(aHash) {
		t.Error("r1 never learned path to A")
	}
	if !r2.HasPath(aHash) {
		t.Error("r2 never learned path to A")
	}
}

// Go persists a Python-announced ratchet; a fresh process reading the
// same storage path must recover the key without a new announce.
func TestLiveInteropRatchetPersistsAcrossRestart(t *testing.T) {
	liveOrSkip(t)

	// Fresh-process half: invoked as a child run so the in-memory ratchet
	// table is empty and only the disk path can satisfy GetRatchet.
	if os.Getenv("INTEROP_RATCHEL_HELPER") == "1" {
		destHash, err := hex.DecodeString(os.Getenv("INTEROP_DEST_HASH"))
		if err != nil {
			t.Fatalf("bad helper hash: %v", err)
		}
		fmt.Printf("RATCHET %s\n", hex.EncodeToString(identity.GetRatchet(destHash)))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	storageDir := t.TempDir()
	t.Setenv("RETICULUM_STORAGE_PATH", storageDir)

	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)

	pyCmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "ratchet_peer.py"))
	pyCmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
	)
	pyCmd.Stderr = os.Stderr
	pyOut, err := pyCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if err := pyCmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	defer func() {
		_ = pyCmd.Process.Kill()
		_ = pyCmd.Wait()
	}()

	br := bufio.NewReader(pyOut)
	if line, err := readLineTimeout(ctx, br, 25*time.Second); err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("READY: %q %v", line, err)
	}
	hashLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	pyDestHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(pyDestHash) != 16 {
		t.Fatalf("bad dest hash %q", hashLine)
	}

	deadline := time.Now().Add(20 * time.Second)
	for !tr.HasPath(pyDestHash) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if !tr.HasPath(pyDestHash) {
		t.Fatal("announce never arrived")
	}
	cleanup()

	ratchet := identity.GetRatchet(pyDestHash)
	if len(ratchet) == 0 {
		t.Fatal("ratchet not remembered after announce")
	}

	// Simulate restart: re-run this test in a fresh child process with an
	// empty in-memory table but the same storage path.
	helper := exec.CommandContext(ctx, os.Args[0],
		"-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=60s")
	helper.Env = append(os.Environ(),
		"INTEROP_RATCHEL_HELPER=1",
		"RUN_LIVE_INTEROP=1",
		"RETICULUM_STORAGE_PATH="+storageDir,
		"INTEROP_DEST_HASH="+hex.EncodeToString(pyDestHash),
	)
	out, err := helper.CombinedOutput()
	if err != nil {
		t.Fatalf("helper run: %v %s", err, out)
	}
	want := "RATCHET " + hex.EncodeToString(ratchet)
	if !strings.Contains(string(out), want) {
		t.Fatalf("fresh process could not recover ratchet: %s", out)
	}
}

// Remote management: Python links, identifies, requests /status on the
// Go rnstransport.remote.management destination.
func TestLiveInteropRemoteMgmtStatusRequest(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	idFile := filepath.Join(t.TempDir(), "pyid")
	mkID := exec.CommandContext(ctx, pythonExe(), "-c",
		"import RNS,sys; i=RNS.Identity(); i.to_file(sys.argv[1]); print(i.hash.hex())", idFile)
	idHexBytes, err := mkID.CombinedOutput()
	if err != nil {
		t.Fatalf("create python identity: %v %s", err, idHexBytes)
	}
	pyIDHash, err := hex.DecodeString(strings.TrimSpace(string(idHexBytes)))
	if err != nil || len(pyIDHash) != 16 {
		t.Fatalf("bad python id %q", idHexBytes)
	}

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)

	cfg := &common.ReticulumConfig{
		EnableTransport:         true,
		EnableRemoteManagement:  true,
		RemoteManagementAllowed: [][]byte{pyIDHash},
	}
	tr := transport.NewTransport(cfg)
	addr := "127.0.0.1:" + strconv.Itoa(pyForward)
	target := "127.0.0.1:" + strconv.Itoa(pyListen)
	iface, err := interfaces.NewUDPInterface("interop_udp", addr, target, true)
	if err != nil {
		t.Fatalf("udp iface: %v", err)
	}
	if err := tr.RegisterInterface("interop_udp", iface); err != nil {
		t.Fatalf("register iface: %v", err)
	}
	if err := iface.Start(); err != nil {
		t.Fatalf("start iface: %v", err)
	}
	defer tr.Close()

	if err := tr.InitializeRemoteManagement(); err != nil {
		t.Fatalf("init remote mgmt: %v", err)
	}
	mgmtDest := tr.RemoteManagementDestination()
	if mgmtDest == nil {
		t.Fatal("no remote management destination")
	}
	if err := mgmtDest.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce mgmt dest: %v", err)
	}

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "remote_mgmt_client.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_MGMT_HASH="+hex.EncodeToString(mgmtDest.GetHash()),
		"INTEROP_IDENTITY_FILE="+idFile,
	)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	if line, err := readLineTimeout(ctx, br, 20*time.Second); err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("READY: %q %v", line, err)
	}
	if _, err := readLineTimeout(ctx, br, 5*time.Second); err != nil {
		t.Fatalf("id hash line: %v", err)
	}
	if line, err := readLineTimeout(ctx, br, 60*time.Second); err != nil || strings.TrimSpace(line) != "LINK_UP" {
		t.Fatalf("LINK_UP: %q %v", line, err)
	}
	line, err := readLineTimeout(ctx, br, 25*time.Second)
	if err != nil {
		t.Fatalf("RESP_LEN: %v", err)
	}
	got := strings.TrimSpace(line)
	if !strings.HasPrefix(got, "RESP_LEN ") {
		t.Fatalf("expected RESP_LEN, got %q", got)
	}
	fmt.Println("remote mgmt /status response:", got)
}

// Python sends a channel message and must see its envelope receipt
// reach DELIVERED, proving Go emits channel packet proofs on the wire.
func TestLiveInteropGoProvesChannelPackets(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

	inbound := make(chan *link.Link, 1)
	destGo.SetLinkEstablishedCallback(func(lnk any) {
		if l, ok := lnk.(*link.Link); ok && l != nil {
			if ch := l.GetChannel(); ch != nil {
				ch.AddMessageHandler(func(channel.MessageBase) bool { return true })
			}
			select {
			case inbound <- l:
			default:
			}
		}
	})
	if err := destGo.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "channel_proof_client.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_GO_DEST_HASH="+hex.EncodeToString(destGo.GetHash()),
	)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	if line, err := readLineTimeout(ctx, br, 20*time.Second); err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("READY: %q %v", line, err)
	}
	got, err := readLineTimeout(ctx, br, 60*time.Second)
	if err != nil {
		t.Fatalf("PROVED: %v", err)
	}
	if strings.TrimSpace(got) != "SENT" && strings.TrimSpace(got) != "PROVED" {
		t.Fatalf("unexpected line %q", got)
	}
	if strings.TrimSpace(got) == "SENT" {
		got, err = readLineTimeout(ctx, br, 30*time.Second)
		if err != nil || strings.TrimSpace(got) != "PROVED" {
			t.Fatalf("expected PROVED, got %q err=%v", got, err)
		}
	}
}

// Same session as the plain TCP live test, but with KISS framing on
// both sides, including a payload full of KISS escape bytes.
func TestLiveInteropPythonTCPClientKISSGoTCPServer(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	tr := transport.NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	port := freeTCPPort(t)
	srv, err := interfaces.NewTCPServerInterface("go_tcp_kiss", "127.0.0.1", port, true, false, false)
	if err != nil {
		t.Fatalf("tcp kiss server: %v", err)
	}
	if err := tr.RegisterInterface("go_tcp_kiss", srv); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}

	pyCfg := t.TempDir()
	writePythonTCPClientKISSConfig(t, pyCfg, port)
	cmd, _, pyHash := startPythonEchoPeer(t, ctx, pyCfg)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	if err := waitPathOn(ctx, tr, pyHash, "go_tcp_kiss", 40*time.Second); err != nil {
		t.Fatalf("path to python over kiss tcp: %v", err)
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
	lnk := link.NewLink(destOut, tr, srv, func(_ *link.Link) {
		close(established)
	}, nil)
	defer lnk.Teardown()
	echoed := make(chan []byte, 1)
	lnk.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		echoed <- append([]byte(nil), data...)
	})
	if err := lnk.Establish(); err != nil {
		t.Fatalf("establish: %v", err)
	}
	select {
	case <-established:
	case <-time.After(45 * time.Second):
		t.Fatal("link establish timeout over kiss tcp")
	}
	lnk.Start()

	payload := bytes.Repeat([]byte{0xC0, 0xDB, 0xDC, 0xDD, 0x00}, 10)
	if err := lnk.SendPacket(payload); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case got := <-echoed:
		if !bytes.Equal(got, payload) {
			t.Fatalf("kiss echo mismatch: %x", got)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no kiss echo from python over tcp")
	}
}

func writePythonTCPClientKISSConfig(t *testing.T, dir string, targetPort int) {
	t.Helper()
	cfg := strings.Join([]string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = no",
		"loglevel = 4",
		"",
		"[interfaces]",
		"",
		"[[py_tcp_client]]",
		"type = TCPClientInterface",
		"enabled = yes",
		"target_host = 127.0.0.1",
		"target_port = " + strconv.Itoa(targetPort),
		"kiss_framing = yes",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}
