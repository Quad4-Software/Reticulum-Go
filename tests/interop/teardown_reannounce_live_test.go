// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live UDP loopback cross-stack interop. Set RUN_LIVE_INTEROP=1.

package interop

import (
	"bufio"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/announce"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/link"
)

// The Go side tears down an inbound link; Python must observe its
// link.closed callback, proving the teardown packet on the wire.
func TestLiveInteropGoTeardownSeenByPython(t *testing.T) {
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
			select {
			case inbound <- l:
			default:
			}
		}
	})
	if err := destGo.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "link_teardown_client.py"))
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
	if err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q err=%v", line, err)
	}
	line, err = readLineTimeout(ctx, br, 60*time.Second)
	if err != nil || strings.TrimSpace(line) != "LINK_UP" {
		t.Fatalf("expected LINK_UP, got %q err=%v", line, err)
	}

	var l *link.Link
	select {
	case l = <-inbound:
	case <-time.After(10 * time.Second):
		t.Fatal("no inbound link arrived at Go destination")
	}
	l.Teardown()

	line, err = readLineTimeout(ctx, br, 15*time.Second)
	if err != nil {
		t.Fatalf("waiting for LINK_CLOSED: %v", err)
	}
	if strings.TrimSpace(line) != "LINK_CLOSED" {
		t.Fatalf("expected LINK_CLOSED, got %q", line)
	}
}

// Python announces the same destination twice with different app_data;
// Go must receive both emissions in order (re-announce update path).
func TestLiveInteropGoSeesPythonReannounceUpdate(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	type seen struct {
		destHash string
		appData  string
	}
	var mu sync.Mutex
	var got []seen
	tr.RegisterAnnounceHandler(&aspectCollector{
		aspect: "linksvc",
		fn: func(destHash []byte, ident any, appData []byte, hops uint8) {
			mu.Lock()
			got = append(got, seen{hex.EncodeToString(destHash), string(appData)})
			mu.Unlock()
		},
	})

	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "reannounce_peer.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_APP_DATA_1=live-va",
		"INTEROP_APP_DATA_2=live-vb",
		"INTEROP_REANNOUNCE_GAP=2",
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
	if err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q err=%v", line, err)
	}
	destHexLine, err := readLineTimeout(ctx, br, 10*time.Second)
	if err != nil {
		t.Fatalf("expected dest hash, err=%v", err)
	}
	wantDest := strings.TrimSpace(destHexLine)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) < 2 {
		t.Fatalf("expected 2 announces, got %d: %+v", len(got), got)
	}
	if got[0].destHash != wantDest || got[1].destHash != wantDest {
		t.Fatalf("announce dest mismatch: %+v want %s", got, wantDest)
	}
	if got[0].appData != "live-va" || got[1].appData != "live-vb" {
		t.Fatalf("announce app_data order wrong: %+v", got)
	}
}

type aspectCollector struct {
	aspect string
	fn     func(destHash []byte, ident any, appData []byte, hops uint8)
}

func (c *aspectCollector) AspectFilter() []string {
	return []string{c.aspect}
}

func (c *aspectCollector) ReceivePathResponses() bool { return false }

func (c *aspectCollector) ReceivedAnnounce(destHash []byte, ident any, appData []byte, hops uint8) error {
	c.fn(destHash, ident, appData, hops)
	return nil
}

var _ announce.Handler = (*aspectCollector)(nil)
