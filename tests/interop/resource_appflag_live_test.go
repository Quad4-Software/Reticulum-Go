// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

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

	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	rlink "github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/resource"
)

// Go -> Python: resource with AppFlag set must arrive with app_flag true.
func TestLiveInteropGoResourceAppFlagToPython(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
		"INTEROP_RESOURCE_EXPECT=interop-appflag-payload",
	)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 25*time.Second)
	if err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("wait READY: %v %q", err, line)
	}
	hashLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("wait hash: %v", err)
	}
	pyHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(pyHash) != 16 {
		t.Fatalf("bad python hash: %q", hashLine)
	}
	if err := waitPath(ctx, tr, pyHash, 40*time.Second); err != nil {
		t.Fatalf("path: %v", err)
	}
	srvID, err := identity.Recall(pyHash)
	if err != nil {
		t.Fatal(err)
	}
	destOut, err := destination.FromHash(pyHash, srvID, destination.Single, tr)
	if err != nil {
		t.Fatal(err)
	}
	established := make(chan struct{})
	lnk := rlink.NewLink(destOut, tr, iface, func(_ *rlink.Link) { close(established) }, nil)
	defer lnk.Teardown()
	lnk.SetPacketCallback(func(_ []byte, _ *packet.Packet) {})
	if err := lnk.Establish(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-established:
	case <-time.After(45 * time.Second):
		t.Fatal("establish timeout")
	}
	lnk.Start()

	res, err := resource.New([]byte("interop-appflag-payload"), false)
	if err != nil {
		t.Fatal(err)
	}
	res.SetAppFlag(true)
	if err := lnk.SendResource(res); err != nil {
		t.Fatal(err)
	}
	line, err = readLineTimeout(ctx, br, 90*time.Second)
	if err != nil {
		t.Fatalf("wait RESOURCE_OK: %v", err)
	}
	if strings.TrimSpace(line) != "RESOURCE_OK" {
		t.Fatalf("expected RESOURCE_OK, got %q", line)
	}
	line, err = readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("wait RESOURCE_APP_FLAG: %v", err)
	}
	if strings.TrimSpace(line) != "RESOURCE_APP_FLAG 1" {
		t.Fatalf("expected RESOURCE_APP_FLAG 1, got %q", line)
	}
}

// Python -> Go: python resource with app_flag=True must arrive with
// IncomingResource.AppFlag true and intact data.
func TestLiveInteropPythonResourceAppFlagToGo(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoUDPPeer(t, pyListen, pyForward)
	defer cleanup()

	idGo, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destGo, err := destination.New(idGo, destination.In, destination.Single, interopApp, tr, interopAspect)
	if err != nil {
		t.Fatal(err)
	}
	destGo.AcceptsLinks(true)

	type gotRes struct {
		data    []byte
		appFlag bool
	}
	resCh := make(chan gotRes, 1)
	var once sync.Once
	destGo.SetLinkEstablishedCallback(func(v any) {
		lnk := interopLink(t, v)
		_ = lnk.SetResourceStrategy(rlink.AcceptAll)
		lnk.SetResourceConcludedCallback(func(p any) {
			switch v := p.(type) {
			case rlink.IncomingResource:
				once.Do(func() { resCh <- gotRes{v.Data, v.AppFlag} })
			case []byte:
				once.Do(func() { resCh <- gotRes{v, false} })
			}
		})
		lnk.Start()
	})
	if err := destGo.Announce(false, nil, nil); err != nil {
		t.Fatal(err)
	}

	script := pyScript(t, "link_client.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_GO_DEST_HASH="+hex.EncodeToString(destGo.GetHash()),
		"INTEROP_LINK_CLIENT_MODE=resource_send",
		"INTEROP_RESOURCE_SEND=interop-appflag-py-payload",
		"INTEROP_RESOURCE_APP_FLAG=1",
	)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 30*time.Second)
	if err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("wait READY: %v %q", err, line)
	}

	select {
	case r := <-resCh:
		if string(r.data) != "interop-appflag-py-payload" {
			t.Fatalf("payload mismatch: %q", r.data)
		}
		if !r.appFlag {
			t.Fatal("expected AppFlag true on incoming resource")
		}
	case <-time.After(60 * time.Second):
		t.Fatal("no resource received")
	}
}
