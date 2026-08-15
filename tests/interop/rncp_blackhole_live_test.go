// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live rncp file transfer and blackhole LINKIDENTIFY interop.
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

	"quad4/reticulum-go/pkg/blackhole"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	rlink "quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/resource"
)

// Go sends a file resource to a Python rncp.receive peer.
func TestLiveInteropGoRNCPToPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, iface, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	script := pyScript(t, "rncp_listen.py")
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
		t.Fatalf("path to python: %v", err)
	}

	idSend, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	srvID, err := identity.Recall(pyHash)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	destOut, err := destination.FromHash(pyHash, srvID, destination.Single, tr)
	if err != nil {
		t.Fatal(err)
	}
	established := make(chan struct{})
	lnk := rlink.NewLink(destOut, tr, iface, func(_ *rlink.Link) { close(established) }, nil)
	if err := lnk.Establish(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-established:
	case <-time.After(45 * time.Second):
		t.Fatal("link establish timeout")
	}
	lnk.Start()
	defer lnk.Teardown()
	if err := lnk.Identify(idSend); err != nil {
		t.Fatalf("identify: %v", err)
	}

	body := bytes.Repeat([]byte("R"), 1024)
	res, err := resource.New(body, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.SetMetadata(map[string]any{"name": []byte("interop.bin")})
	if err := lnk.SendResource(res); err != nil {
		t.Fatalf("send resource: %v", err)
	}

	line, err = readLineTimeout(ctx, br, 90*time.Second)
	if err != nil {
		t.Fatalf("wait FILE_OK: %v", err)
	}
	if strings.TrimSpace(line) != "FILE_OK" {
		t.Fatalf("expected FILE_OK, got %q", line)
	}
}

// Go is the link responder. Python initiates and calls identify(). Python only
// sends LINKIDENTIFY as initiator so roles must match RNS semantics. Go
// blackholes the Python identity hash then tears the link down on identify.
func TestLiveInteropBlackholeLinkIdentifyTeardown(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	localID, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	blackhole.SetLocalIdentityHash(localID.Hash())
	tab := blackhole.New("")
	tr.SetBlackholeTable(tab)

	idGo, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destGo, err := destination.New(idGo, destination.In, destination.Single, interopApp, tr, interopAspect)
	if err != nil {
		t.Fatal(err)
	}
	destGo.AcceptsLinks(true)

	established := make(chan struct{})
	closed := make(chan struct{}, 1)
	destGo.SetLinkEstablishedCallback(func(v any) {
		lnk := interopLink(t, v)
		lnk.SetLinkClosedCallback(func(_ *rlink.Link) {
			select {
			case closed <- struct{}{}:
			default:
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
		"INTEROP_LINK_CLIENT_MODE=identify",
	)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
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
	idLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("wait IDHASH: %v", err)
	}
	idLine = strings.TrimSpace(idLine)
	if !strings.HasPrefix(idLine, "IDHASH ") {
		t.Fatalf("expected IDHASH line, got %q", idLine)
	}
	pyIDHash, err := hex.DecodeString(strings.TrimPrefix(idLine, "IDHASH "))
	if err != nil || len(pyIDHash) != 16 {
		t.Fatalf("bad identity hash: %q err %v", idLine, err)
	}
	if _, err := tab.Add(pyIDHash, 0, "interop blackhole"); err != nil {
		t.Fatalf("blackhole add: %v", err)
	}
	if _, err := stdin.Write([]byte("PROCEED\n")); err != nil {
		t.Fatalf("proceed: %v", err)
	}

	select {
	case <-established:
	case <-time.After(90 * time.Second):
		t.Fatal("incoming link establish timeout")
	}

	select {
	case <-closed:
	case <-time.After(30 * time.Second):
		t.Fatal("expected link teardown after blackholed LINKIDENTIFY")
	}
}
