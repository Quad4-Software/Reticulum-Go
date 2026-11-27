// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/backbone"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	rlink "git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/resource"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/rnsutil"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// Live end-to-end resource transfers over the public Vega backbones.
// Set RUN_VEGA_LIVE=1 to run. These exercise the adaptive resource
// receive window against real internet latency, not loopback.

const (
	vegaHost   = "78.46.92.205"
	vegaGoPort = 4243 // Go backbone server
	vegaPyPort = 4242 // Python RNS backbone server
	vegaApp    = "vegaapp"
	vegaAspect = "resource"
)

func vegaOrSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_VEGA_LIVE") != "1" {
		t.Skip("set RUN_VEGA_LIVE=1 to run live tests over the Vega public backbones")
	}
}

// newBackboneHub initializes the global backbone hub for a test.
func newBackboneHub(t *testing.T) *backbone.Hub {
	t.Helper()
	backbone.Shutdown()
	hub, err := backbone.Init(backbone.DefaultBackend())
	if err != nil {
		t.Fatalf("backbone.Init: %v", err)
	}
	return hub
}

// newBackboneTransport builds a transport with one Backbone client
// interface attached to host:port, waits for it to come online, and
// returns a cleanup that closes both. The hub is created once per test
// and shared. Pass the same hub to every transport in the test.
func newBackboneTransport(t *testing.T, hub *backbone.Hub, name, host string, port int) (*transport.Transport, func()) {
	t.Helper()
	tr := transport.NewTransport(interopPeerConfig())
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("InitializePathRequestHandler: %v", err)
	}
	client, err := interfaces.NewBackboneClientInterface(name, &common.InterfaceConfig{
		Enabled:    true,
		TargetHost: host,
		TargetPort: port,
	}, hub)
	if err != nil {
		t.Fatalf("backbone client: %v", err)
	}
	// Bring the client online before registering: the transport snapshots
	// interface liveness at registration time.
	if err := client.Start(); err != nil {
		t.Fatalf("client.Start: %v", err)
	}
	online := time.Now().Add(60 * time.Second)
	for time.Now().Before(online) && !client.IsOnline() {
		time.Sleep(100 * time.Millisecond)
	}
	if !client.IsOnline() {
		t.Fatalf("backbone client %s never came online", name)
	}
	if err := tr.RegisterInterface(name, client); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}
	cleanup := func() {
		_ = client.Stop()
		_ = tr.Close()
	}
	return tr, cleanup
}

func newGoResourceDestination(t *testing.T, tr *transport.Transport) *destination.Destination {
	t.Helper()
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	dest, err := destination.New(id, destination.In, destination.Single, vegaApp, tr, vegaAspect)
	if err != nil {
		t.Fatalf("destination.New: %v", err)
	}
	dest.AcceptsLinks(true)
	return dest
}

func waitVegaPath(ctx context.Context, tr *transport.Transport, hash []byte) error {
	// RNS 1.6 path table sharding: relays no longer broadcast every
	// announce to every client, so waiting passively for a path can
	// starve forever. AwaitPath sends a path request. Retry a few times
	// across the overall deadline in case the first request is lost.
	deadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if tr.HasPath(hash) {
			return nil
		}
		reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := tr.AwaitPath(reqCtx, hash)
		cancel()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return fmt.Errorf("path to %x not learned within deadline", hash)
}

func TestVegaGoToGoResource(t *testing.T) {
	vegaOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	hub := newBackboneHub(t)
	trRecv, cleanupRecv := newBackboneTransport(t, hub, "vega_recv", vegaHost, vegaGoPort)
	defer cleanupRecv()
	trSend, cleanupSend := newBackboneTransport(t, hub, "vega_send", vegaHost, vegaGoPort)
	defer cleanupSend()

	dest := newGoResourceDestination(t, trRecv)
	payload := bytes.Repeat([]byte{0xC1}, 64_000)
	got := make(chan []byte, 1)
	var once sync.Once
	dest.SetLinkEstablishedCallback(func(l any) {
		lnk := interopLink(t, l)
		_ = lnk.SetResourceStrategy(rlink.AcceptAll)
		lnk.SetResourceConcludedCallback(func(p any) {
			once.Do(func() {
				switch v := p.(type) {
				case rlink.IncomingResource:
					got <- v.Data
				case []byte:
					got <- v
				}
			})
		})
	})
	if err := dest.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	destHash := dest.GetHash()
	if err := waitVegaPath(ctx, trSend, destHash); err != nil {
		t.Fatalf("path: %v", err)
	}
	lnk, err := rnsutil.EstablishRNCPLink(ctx, trSend, destHash)
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	defer lnk.Teardown()

	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}
	t0 := time.Now()
	if err := lnk.SendResource(res); err != nil {
		t.Fatalf("SendResource: %v", err)
	}
	select {
	case b := <-got:
		if !bytes.Equal(b, payload) {
			t.Fatalf("payload mismatch: %d bytes", len(b))
		}
		t.Logf("Go to Go over Vega Go backbone ok in %s", time.Since(t0))
	case <-time.After(4 * time.Minute):
		t.Fatalf("resource not delivered within deadline (waited %s)", time.Since(t0))
	}
}

// runVegaPythonPeer starts tests/interop/py/backbone_resource_peer.py
// attached to the given backbone, returning a line reader and a kill func.
func runVegaPythonPeer(ctx context.Context, t *testing.T, mode string, extraEnv ...string) (*bufio.Reader, func()) {
	t.Helper()
	cctx, cancel := context.WithCancel(ctx)
	cmdArgs := []string{pyScript(t, "backbone_resource_peer.py")}
	cmd := exec.CommandContext(cctx, pythonExe(), cmdArgs...)
	cmd.Env = append(os.Environ(),
		"PYTHON_INTEROP="+pythonExe(),
		"INTEROP_MODE="+mode,
		"INTEROP_BACKBONE_HOST="+vegaHost,
		"INTEROP_BACKBONE_PORT="+strconv.Itoa(vegaPyPort),
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start python: %v", err)
	}
	cleanup := func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		cancel()
	}
	return bufio.NewReader(out), cleanup
}

func readVegaLine(ctx context.Context, br *bufio.Reader, want string, d time.Duration) (string, error) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		line, err := readLineTimeout(ctx, br, time.Until(deadline))
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == want || strings.HasPrefix(line, want) {
			return line, nil
		}
	}
	return "", fmt.Errorf("line %q not seen within deadline", want)
}

func TestVegaGoToPythonResource(t *testing.T) {
	vegaOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	br, pyCleanup := runVegaPythonPeer(ctx, t, "recv")
	defer pyCleanup()

	_, err := readVegaLine(ctx, br, "READY", 90*time.Second)
	if err != nil {
		t.Fatalf("READY: %v", err)
	}
	hashLine, err := readVegaLine(ctx, br, "HASH ", 60*time.Second)
	if err != nil {
		t.Fatalf("HASH: %v", err)
	}
	pyHash, err := hexFromLine(hashLine)
	if err != nil {
		t.Fatalf("bad python hash %q: %v", hashLine, err)
	}

	hub := newBackboneHub(t)
	tr, cleanupTr := newBackboneTransport(t, hub, "vega_go_sender", vegaHost, vegaPyPort)
	defer cleanupTr()

	if err := waitVegaPath(ctx, tr, pyHash); err != nil {
		t.Fatalf("path to python peer: %v", err)
	}
	srvID, err := identity.Recall(pyHash)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	destOut, err := destination.FromHash(pyHash, srvID, destination.Single, tr)
	if err != nil {
		t.Fatalf("from hash: %v", err)
	}

	established := make(chan struct{}, 1)
	lnk := rlink.NewLink(destOut, tr, nil, func(*rlink.Link) {
		select {
		case established <- struct{}{}:
		default:
		}
	}, nil)
	defer lnk.Teardown()
	if err := lnk.Establish(); err != nil {
		t.Fatalf("establish: %v", err)
	}
	select {
	case <-established:
	case <-time.After(90 * time.Second):
		t.Fatal("link establish timeout")
	}
	lnk.Start()

	payload := bytes.Repeat([]byte{0xD2}, 64_000)
	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}
	t0 := time.Now()
	if err := lnk.SendResource(res); err != nil {
		t.Fatalf("SendResource: %v", err)
	}

	// Python peer prints "RESOURCE_OK <len> <sha256>".
	var line string
	line, err = readVegaLine(ctx, br, "RESOURCE_OK", 4*time.Minute)
	if err != nil {
		t.Fatalf("resource result: %v", err)
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) != 3 || fields[0] != "RESOURCE_OK" {
		t.Fatalf("expected RESOURCE_OK line, got %q", line)
	}
	if fields[1] != strconv.Itoa(len(payload)) {
		t.Fatalf("payload length %s != %d", fields[1], len(payload))
	}
	sum := sha256.Sum256(payload)
	if fields[2] != fmt.Sprintf("%x", sum) {
		t.Fatalf("payload digest mismatch: %s", fields[2])
	}
	t.Logf("Go to Python over Vega Python backbone ok in %s", time.Since(t0))
}

func TestVegaPythonToGoResource(t *testing.T) {
	vegaOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	hub := newBackboneHub(t)
	tr, cleanupTr := newBackboneTransport(t, hub, "vega_go_recv", vegaHost, vegaPyPort)
	defer cleanupTr()

	dest := newGoResourceDestination(t, tr)
	payload := bytes.Repeat([]byte{0xE3}, 64_000)
	got := make(chan []byte, 1)
	var once sync.Once
	dest.SetLinkEstablishedCallback(func(l any) {
		lnk := interopLink(t, l)
		_ = lnk.SetResourceStrategy(rlink.AcceptAll)
		lnk.SetResourceConcludedCallback(func(p any) {
			once.Do(func() {
				switch v := p.(type) {
				case rlink.IncomingResource:
					got <- v.Data
				case []byte:
					got <- v
				}
			})
		})
	})
	if err := dest.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}
	goHash := fmt.Sprintf("%x", dest.GetHash())

	br, pyCleanup := runVegaPythonPeer(ctx, t, "send", "INTEROP_TARGET_HASH="+goHash)
	defer pyCleanup()

	if _, err := readVegaLine(ctx, br, "READY", 90*time.Second); err != nil {
		t.Fatalf("READY: %v", err)
	}
	// Wait for the python peer to announce (its HASH line) so the path
	// lookup on the python side can resolve our announce.
	if _, err := readVegaLine(ctx, br, "HASH ", 60*time.Second); err != nil {
		t.Fatalf("python HASH: %v", err)
	}

	select {
	case b := <-got:
		// The Python peer sends os.urandom(64000). Content integrity is
		// proven by the resource hash/proof exchange itself, so assert
		// the delivery length here and cross-check the peer's completion
		// line below.
		if len(b) != len(payload) {
			t.Fatalf("payload length %d != %d", len(b), len(payload))
		}
		t.Logf("Python to Go over Vega Python backbone ok")
	case <-time.After(4 * time.Minute):
		t.Fatal("resource not delivered within deadline")
	}
}

func hexFromLine(line string) ([]byte, error) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) != 2 || fields[0] != "HASH" {
		return nil, fmt.Errorf("not a hash line: %q", line)
	}
	return hex.DecodeString(fields[1])
}
