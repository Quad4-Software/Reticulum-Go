// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live TCP server interop: a Python TCPClientInterface session against a Go
// TCPServerInterface carrying announces and link traffic.
// Set RUN_LIVE_INTEROP=1 to enable.

//go:build !js

package interop

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	rlink "github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

// waitPathOn is waitPath parameterized by the interface name to request on.
func waitPathOn(ctx context.Context, tr *transport.Transport, destHash []byte, ifaceName string, total time.Duration) error {
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
		_ = tr.RequestPath(destHash, ifaceName, nil, false)
		time.Sleep(80 * time.Millisecond)
	}
	return context.DeadlineExceeded
}

// writePythonTCPClientConfig writes a Python Reticulum config whose only
// interface is a TCPClientInterface pointing at the Go TCP server port.
func writePythonTCPClientConfig(t *testing.T, dir string, targetPort int) {
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
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLiveInteropPythonTCPClientGoTCPServer runs a full RNS session over a
// Python TCP client connected to a Go TCP server: announces both ways, a Go
// initiated link echo, and a Python initiated link echo.
func TestLiveInteropPythonTCPClientGoTCPServer(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), pyProcShortTimeout)
	defer cancel()

	tr := transport.NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	port := freeTCPPort(t)
	srv, err := interfaces.NewTCPServerInterface("go_tcp_srv", "127.0.0.1", port, false, false, false)
	if err != nil {
		t.Fatalf("tcp server iface: %v", err)
	}
	if err := tr.RegisterInterface("go_tcp_srv", srv); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}

	// Go responder destination for the Python-initiated link.
	idGo, err := identity.New()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	destGo, err := destination.New(idGo, destination.In, destination.Single, interopApp, tr, interopAspect)
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	destGo.AcceptsLinks(true)
	goEchoed := make(chan struct{})
	destGo.SetLinkEstablishedCallback(func(v any) {
		lnk := interopLink(t, v)
		lnk.SetPacketCallback(func(data []byte, _ *packet.Packet) {
			if err := lnk.SendPacket(data); err == nil {
				close(goEchoed)
			}
		})
		lnk.Start()
	})
	if err := destGo.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	pyCfg := t.TempDir()
	writePythonTCPClientConfig(t, pyCfg, port)
	cmd, _, pyHash := startPythonEchoPeer(t, ctx, pyCfg,
		"INTEROP_CLIENT_TO="+hex.EncodeToString(destGo.GetHash()))
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Python announce should arrive over the TCP session.
	if err := waitPathOn(ctx, tr, pyHash, "go_tcp_srv", 40*time.Second); err != nil {
		t.Fatalf("path to python over tcp server: %v", err)
	}

	// Go initiates a link to the Python destination and echoes.
	srvID, err := identity.Recall(pyHash)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	destOut, err := destination.FromHash(pyHash, srvID, destination.Single, tr)
	if err != nil {
		t.Fatalf("from hash: %v", err)
	}
	established := make(chan struct{})
	lnk := rlink.NewLink(destOut, tr, srv, func(_ *rlink.Link) {
		close(established)
	}, nil)
	defer lnk.Teardown()
	echoed := make(chan struct{})
	lnk.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		close(echoed)
	})
	if err := lnk.Establish(); err != nil {
		t.Fatalf("establish: %v", err)
	}
	select {
	case <-established:
	case <-time.After(45 * time.Second):
		t.Fatal("link establish timeout over tcp")
	}
	lnk.Start()
	payload := []byte("go-tcp-echo")
	if err := lnk.SendPacket(payload); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case <-echoed:
	case <-time.After(30 * time.Second):
		t.Fatal("no echo from python over tcp")
	}

	// Python initiated link echo arrives via INTEROP_CLIENT_TO.
	select {
	case <-goEchoed:
	case <-time.After(60 * time.Second):
		t.Fatal("python client link echo not received")
	}
}
