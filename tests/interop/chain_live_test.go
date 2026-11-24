// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bufio"
	"context"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/announce"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/backbone"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

const chainApp = "interop_chain"

// countHandler counts ReceivedAnnounce calls per destination hash.
type countHandler struct {
	aspects []string
	counts  sync.Map
}

func (h *countHandler) AspectFilter() []string     { return h.aspects }
func (h *countHandler) ReceivePathResponses() bool { return true }
func (h *countHandler) ReceivedAnnounce(destHash []byte, _ any, _ []byte, _ uint8) error {
	key := hex.EncodeToString(destHash)
	v, _ := h.counts.LoadOrStore(key, new(int64))
	atomic.AddInt64(v.(*int64), 1)
	return nil
}

func (h *countHandler) count(destHash []byte) int64 {
	if v, ok := h.counts.Load(hex.EncodeToString(destHash)); ok {
		return atomic.LoadInt64(v.(*int64))
	}
	return 0
}

var _ announce.Handler = (*countHandler)(nil)

// chainTransport returns a transport-enabled node for chain tests.
func chainTransport(t *testing.T) *transport.Transport {
	t.Helper()
	cfg := interopPeerConfig()
	cfg.EnableTransport = true
	return transport.NewTransport(cfg)
}

type spawnSet struct {
	mu   sync.Mutex
	list []*interfaces.BackboneClientInterface
}

func (s *spawnSet) add(c *interfaces.BackboneClientInterface) {
	s.mu.Lock()
	s.list = append(s.list, c)
	s.mu.Unlock()
}

func (s *spawnSet) snapshot() []*interfaces.BackboneClientInterface {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*interfaces.BackboneClientInterface(nil), s.list...)
}

// goBackboneServer starts a Go backbone listener registered to tr and returns
// the bound port plus a spawn set of accepted client interfaces.
func goBackboneServer(t *testing.T, tr *transport.Transport, hub *backbone.Hub, name string, spawned *spawnSet) int {
	t.Helper()
	port := freeTCPPort(t)
	srv, err := interfaces.NewBackboneInterface(name, &common.InterfaceConfig{
		Enabled: true,
		Address: "127.0.0.1",
		Port:    port,
	}, hub, spawned.add)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(name, srv); err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	return port
}

// startChainPy spawns chain_node.py and returns the process, stdout reader
// and stdin writer.
func startChainPy(t *testing.T, ctx context.Context, env []string) (*exec.Cmd, *bufio.Reader, io.Writer) {
	t.Helper()
	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "chain_node.py"))
	cmd.Env = append(os.Environ(), env...)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd, bufio.NewReader(out), in
}

func killChainPy(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// readDest reads READY and DEST lines, returning the destination hash.
func readDest(t *testing.T, br *bufio.Reader) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	line, err := readLineTimeout(ctx, br, 30*time.Second)
	if err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("READY: %q %v", line, err)
	}
	line, err = readLineTimeout(ctx, br, 10*time.Second)
	if err != nil {
		t.Fatalf("DEST: %v", err)
	}
	h := strings.TrimSpace(line)
	if !strings.HasPrefix(h, "DEST ") {
		t.Fatalf("expected DEST, got %q", h)
	}
	dh, err := hex.DecodeString(strings.TrimPrefix(h, "DEST "))
	if err != nil {
		t.Fatal(err)
	}
	return dh
}

func chainEndEnv(port int, aspect string, announce bool) []string {
	env := []string{
		"CHAIN_ROLE=end",
		"PORT=" + strconv.Itoa(port),
		"APP=" + chainApp,
		"ASPECT=" + aspect,
	}
	if announce {
		env = append(env, "ANNOUNCE=1")
	}
	return env
}

func pumpLines(br *bufio.Reader, ch chan<- string) {
	for {
		l, err := br.ReadString('\n')
		if err != nil {
			return
		}
		ch <- strings.TrimSpace(l)
	}
}

// waitPyHops waits until the Python end reports exactly wantHops for destHex.
func waitPyHops(t *testing.T, lines <-chan string, destHex string, wantHops int, total time.Duration) {
	t.Helper()
	deadline := time.Now().Add(total)
	for time.Now().Before(deadline) {
		select {
		case l := <-lines:
			f := strings.Fields(l)
			if len(f) == 3 && f[0] == "PATH" && f[1] == destHex {
				got, _ := strconv.Atoi(f[2])
				if got == wantHops {
					return
				}
				t.Fatalf("path %s reported %d hops, want %d", destHex[:16], got, wantHops)
			}
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatalf("no PATH report for %s within %s", destHex[:16], total)
}

// waitGoHops polls the Go path table until destHash has exactly wantHops.
func waitGoHops(t *testing.T, tr *transport.Transport, destHash []byte, wantHops int, total time.Duration) {
	t.Helper()
	deadline := time.Now().Add(total)
	for time.Now().Before(deadline) {
		if tr.HasPath(destHash) && int(tr.HopsTo(destHash)) == wantHops {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no path at %d hops for %x", wantHops, destHash[:8])
}

// TestLiveInteropChainPythonGoPython runs a Python-Go-Python backbone chain.
// The Go middle node relays announces at the right hop count, answers path
// requests only toward the requestor interface, and emits no ghost announces
// after an endpoint goes offline.
func TestLiveInteropChainPythonGoPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	backbone.Shutdown()
	hub, err := backbone.Init(backbone.DefaultBackend())
	if err != nil {
		t.Fatal(err)
	}
	defer backbone.Shutdown()

	trB := chainTransport(t)
	defer func() { _ = trB.Close() }()
	if err := trB.InitializePathRequestHandler(); err != nil {
		t.Fatal(err)
	}

	var spawnedA, spawnedC spawnSet
	pA := goBackboneServer(t, trB, hub, "chain_a", &spawnedA)
	pC := goBackboneServer(t, trB, hub, "chain_c", &spawnedC)

	_, rA, inA := startChainPy(t, ctx, chainEndEnv(pA, "a", false))
	dA := readDest(t, rA)
	cmdC, rC, inC := startChainPy(t, ctx, chainEndEnv(pC, "c", false))
	dC := readDest(t, rC)

	linesA := make(chan string, 64)
	linesC := make(chan string, 64)
	go pumpLines(rA, linesA)
	go pumpLines(rC, linesC)

	dAhex := hex.EncodeToString(dA)
	dChex := hex.EncodeToString(dC)

	// Announce from each end and let the relay propagate at hops=2.
	_, _ = io.WriteString(inC, "WATCH "+dAhex+"\n")
	_, _ = io.WriteString(inA, "WATCH "+dChex+"\n")
	_, _ = io.WriteString(inA, "ANNOUNCE\n")
	_, _ = io.WriteString(inC, "ANNOUNCE\n")

	waitPyHops(t, linesC, dAhex, 2, 90*time.Second)
	waitPyHops(t, linesA, dChex, 2, 90*time.Second)

	// PATH_RESPONSE direction: C requests dA. The Go relay must send the
	// answer only on the interface toward C, never toward A. Let announce
	// retransmission settle first, then snapshot counters.
	time.Sleep(15 * time.Second)
	spawnA := spawnedA.snapshot()
	spawnC := spawnedC.snapshot()
	if len(spawnA) != 1 || len(spawnC) != 1 {
		t.Fatalf("spawned ifaces: A=%d C=%d want 1/1", len(spawnA), len(spawnC))
	}
	txA0 := spawnA[0].GetTxBytes()
	txC0 := spawnC[0].GetTxBytes()

	_, _ = io.WriteString(inC, "REQUEST "+dAhex+"\n")
	time.Sleep(8 * time.Second)

	txAd := spawnA[0].GetTxBytes() - txA0
	txCd := spawnC[0].GetTxBytes() - txC0
	if txAd != 0 {
		t.Fatalf("PATH_RESPONSE leaked toward unrelated interface: txA delta=%d", txAd)
	}
	if txCd == 0 {
		t.Fatalf("no PATH_RESPONSE sent toward requestor: txC delta=%d", txCd)
	}

	// Ghost check: take C offline. The relay must not spontaneously emit
	// announces for dC (or anything else) toward A afterward.
	txA1 := spawnA[0].GetTxBytes()
	killChainPy(cmdC)
	time.Sleep(12 * time.Second)
	if d := spawnA[0].GetTxBytes() - txA1; d != 0 {
		t.Fatalf("ghost traffic toward A after C offline: txA delta=%d", d)
	}

	// A path request for the now-offline dC may still be answered from the
	// relay's cached announce (Python answers from cache too) but only
	// toward the requestor.
	txA2 := spawnA[0].GetTxBytes()
	txC2 := spawnC[0].GetTxBytes()
	_, _ = io.WriteString(inA, "REQUEST "+dChex+"\n")
	time.Sleep(8 * time.Second)
	if d := spawnA[0].GetTxBytes() - txA2; d == 0 {
		t.Fatal("no cached PATH_RESPONSE toward requestor after C offline")
	}
	if d := spawnC[0].GetTxBytes() - txC2; d != 0 {
		t.Fatalf("PATH_RESPONSE toward dead endpoint interface: delta=%d", d)
	}
}

// TestLiveInteropChainGoPythonGo runs a Go-Python-Go backbone chain. The
// Python middle node relays announces at the right hop count, the Go ends see
// paths at hops=2, and a path request is answered only toward the requestor
// with no ghost announces after an end goes offline.
func TestLiveInteropChainGoPythonGo(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	backbone.Shutdown()
	hub, err := backbone.Init(backbone.DefaultBackend())
	if err != nil {
		t.Fatal(err)
	}
	defer backbone.Shutdown()

	// A: Go server end announcing dA.
	trA := chainTransport(t)
	defer func() { _ = trA.Close() }()
	if err := trA.InitializePathRequestHandler(); err != nil {
		t.Fatal(err)
	}
	var spawnedA spawnSet
	pA := goBackboneServer(t, trA, hub, "chain_a", &spawnedA)

	// B: Python relay, client into A and server for C.
	pB := freeTCPPort(t)
	_, rB, _ := startChainPy(t, ctx, []string{
		"CHAIN_ROLE=relay",
		"UP_PORT=" + strconv.Itoa(pA),
		"DOWN_PORT=" + strconv.Itoa(pB),
	})
	line, err := readLineTimeout(ctx, rB, 30*time.Second)
	if err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("relay READY: %q %v", line, err)
	}

	// C: Go client end announcing dC.
	trC := chainTransport(t)
	if err := trC.InitializePathRequestHandler(); err != nil {
		t.Fatal(err)
	}
	cliC, err := interfaces.NewBackboneClientInterface("chain_c", &common.InterfaceConfig{
		Enabled:    true,
		TargetHost: "127.0.0.1",
		TargetPort: pB,
	}, hub)
	if err != nil {
		t.Fatal(err)
	}
	if err := trC.RegisterInterface("chain_c", cliC); err != nil {
		t.Fatal(err)
	}
	if err := cliC.Start(); err != nil {
		t.Fatal(err)
	}

	destA, dA := goChainEnd(t, trA, "a")
	destC, dC := goChainEnd(t, trC, "c")

	handA := &countHandler{aspects: nil}
	trA.RegisterAnnounceHandler(handA)

	if err := destA.Announce(false, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := destC.Announce(false, nil, nil); err != nil {
		t.Fatal(err)
	}

	waitGoHops(t, trC, dA, 2, 90*time.Second)
	waitGoHops(t, trA, dC, 2, 90*time.Second)

	// A path request from A for dC is answered back along the chain only.
	// C's client interface must see zero new bytes (no leaked response).
	time.Sleep(15 * time.Second)
	rxC0 := cliC.GetRxBytes()
	if err := trA.RequestPath(dC, "", nil, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(8 * time.Second)
	if d := cliC.GetRxBytes() - rxC0; d != 0 {
		t.Fatalf("PATH_RESPONSE leaked toward non-requestor end: rxC delta=%d", d)
	}
	if !trA.HasPath(dC) || int(trA.HopsTo(dC)) != 2 {
		t.Fatalf("path to dC lost after request: has=%v hops=%d", trA.HasPath(dC), trA.HopsTo(dC))
	}

	// Take C offline. No ghost announces for dC may arrive at A.
	base := handA.count(dC)
	_ = trC.Close()
	time.Sleep(15 * time.Second)
	if d := handA.count(dC) - base; d != 0 {
		t.Fatalf("ghost announces for offline dC: +%d", d)
	}
}

func goChainEnd(t *testing.T, tr *transport.Transport, aspect string) (*destination.Destination, []byte) {
	t.Helper()
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(id, destination.In, destination.Single, chainApp, tr, aspect)
	if err != nil {
		t.Fatal(err)
	}
	return dest, dest.GetHash()
}
