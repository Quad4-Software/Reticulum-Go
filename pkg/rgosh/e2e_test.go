// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
)

func TestE2E_RgoshPipeEcho(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	dirA := t.TempDir()
	dirB := t.TempDir()
	writeUDPPeer(t, dirA, portA, portB)
	writeUDPPeer(t, dirB, portB, portA)

	cfgA, err := rnsutil.LoadConfigDir(dirA)
	if err != nil {
		t.Fatal(err)
	}
	cfgB, err := rnsutil.LoadConfigDir(dirB)
	if err != nil {
		t.Fatal(err)
	}
	nA, err := node.New(cfgA)
	if err != nil {
		t.Fatal(err)
	}
	nB, err := node.New(cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if err := nA.Start(); err != nil {
		t.Fatal(err)
	}
	defer nA.Stop()
	if err := nB.Start(); err != nil {
		t.Fatal(err)
	}
	defer nB.Stop()

	idListen, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	idClient, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}

	dest, err := destination.New(idListen, destination.In, destination.Single, rnsutil.RgoshAppName, nA.Transport())
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)

	var stdoutMu sync.Mutex
	var stdoutBuf bytes.Buffer
	exitCh := make(chan int, 1)
	listenerReady := make(chan struct{}, 1)

	dest.SetLinkEstablishedCallback(func(lnk any) {
		l, ok := lnk.(*link.Link)
		if !ok || l == nil {
			return
		}
		ch := l.GetChannel()
		_ = RegisterNative(ch)
		sess := NewSession(Config{
			Listener:   true,
			AllowAll:   true,
			DefaultCmd: []string{"/bin/echo", "hello-rgosh"},
		}, ChannelSender{Ch: ch})
		sess.StartProcess = StartLocalProcess
		sess.OnTeardown = func() { l.Teardown() }
		ch.AddMessageHandler(func(msg Message) bool {
			_ = sess.HandleMessage(msg)
			return true
		})
		select {
		case listenerReady <- struct{}{}:
		default:
		}
	})
	_ = dest.Announce(false, nil, nil)
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	l, err := rnsutil.EstablishRgoshLink(ctx, nB.Transport(), dest.GetHash(), rnsutil.RgoshAppName)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Teardown()

	select {
	case <-listenerReady:
	case <-time.After(5 * time.Second):
		t.Fatal("listener session not ready")
	}

	if err := l.Identify(idClient); err != nil {
		t.Fatal(err)
	}
	ch := l.GetChannel()
	_ = RegisterNative(ch)
	sess := NewSession(Config{Listener: false}, ChannelSender{Ch: ch})
	sess.OnStdout = func(b []byte) {
		stdoutMu.Lock()
		stdoutBuf.Write(b)
		stdoutMu.Unlock()
	}
	sess.OnExit = func(code int) {
		select {
		case exitCh <- code:
		default:
		}
	}
	ch.AddMessageHandler(func(msg Message) bool {
		_ = sess.HandleMessage(msg)
		return true
	})
	time.Sleep(50 * time.Millisecond)
	if err := sess.SendVersion(); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	lastVers := time.Now()
	for sess.State() == StateWaitVers {
		select {
		case <-deadline:
			t.Fatal("version timeout")
		case <-time.After(50 * time.Millisecond):
			if sess.State() == StateWaitVers && time.Since(lastVers) >= 2*time.Second {
				_ = sess.SendVersion()
				lastVers = time.Now()
			}
		}
	}
	if err := sess.SendExec(ExecRequest{
		Cmdline:    []string{"/bin/echo", "hello-rgosh"},
		PipeStdin:  true,
		PipeStdout: true,
		PipeStderr: true,
	}); err != nil {
		t.Fatal(err)
	}
	_ = sess.SendStream(StreamStdin, nil, true)

	select {
	case code := <-exitCh:
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
	case <-time.After(30 * time.Second):
		stdoutMu.Lock()
		out := stdoutBuf.String()
		stdoutMu.Unlock()
		t.Fatalf("exit timeout stdout=%q state=%s", out, sess.State())
	}
	stdoutMu.Lock()
	out := stdoutBuf.String()
	stdoutMu.Unlock()
	if !strings.Contains(out, "hello-rgosh") {
		t.Fatalf("stdout=%q", out)
	}
}

func TestE2E_RgoshAuthDeny(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	dirA := t.TempDir()
	dirB := t.TempDir()
	writeUDPPeer(t, dirA, portA, portB)
	writeUDPPeer(t, dirB, portB, portA)

	cfgA, err := rnsutil.LoadConfigDir(dirA)
	if err != nil {
		t.Fatal(err)
	}
	cfgB, err := rnsutil.LoadConfigDir(dirB)
	if err != nil {
		t.Fatal(err)
	}
	nA, err := node.New(cfgA)
	if err != nil {
		t.Fatal(err)
	}
	nB, err := node.New(cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if err := nA.Start(); err != nil {
		t.Fatal(err)
	}
	defer nA.Stop()
	if err := nB.Start(); err != nil {
		t.Fatal(err)
	}
	defer nB.Stop()

	idListen, _ := identity.New()
	idClient, _ := identity.New()
	idAllowed, _ := identity.New()

	sentinel := filepath.Join(t.TempDir(), "should-not-exist")
	dest, err := destination.New(idListen, destination.In, destination.Single, rnsutil.RgoshAppName, nA.Transport())
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)
	spawned := false
	dest.SetLinkEstablishedCallback(func(lnk any) {
		l := lnk.(*link.Link)
		ch := l.GetChannel()
		_ = RegisterNative(ch)
		sess := NewSession(Config{
			Listener:   true,
			Allowed:    [][]byte{idAllowed.Hash()},
			DefaultCmd: []string{"/bin/touch", sentinel},
		}, ChannelSender{Ch: ch})
		sess.StartProcess = func(req ExecRequest) (ProcessHandle, error) {
			spawned = true
			return StartLocalProcess(req)
		}
		sess.OnTeardown = func() { l.Teardown() }
		ch.AddMessageHandler(func(msg Message) bool {
			_ = sess.HandleMessage(msg)
			return true
		})
		l.SetRemoteIdentifiedCallback(func(lnk *link.Link, remote *identity.Identity) {
			if remote == nil || !sess.SetRemoteIdentity(remote.Hash()) {
				lnk.Teardown()
			}
		})
	})
	_ = dest.Announce(false, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	l, err := rnsutil.EstablishRgoshLink(ctx, nB.Transport(), dest.GetHash(), rnsutil.RgoshAppName)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Teardown()
	_ = l.Identify(idClient)
	time.Sleep(500 * time.Millisecond)
	if spawned {
		t.Fatal("process spawned despite auth deny")
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("sentinel file created")
	}
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func writeUDPPeer(t *testing.T, dir string, listen, peer int) {
	t.Helper()
	cfg := strings.Join([]string{
		"[reticulum]",
		"enable_transport = yes",
		"share_instance = no",
		"",
		"[interfaces]",
		"  [[UDP]]",
		"    type = UDPInterface",
		"    enabled = yes",
		"    listen_ip = 127.0.0.1",
		"    listen_port = " + strconv.Itoa(listen),
		"    target_host = 127.0.0.1",
		"    target_port = " + strconv.Itoa(peer),
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

var _ = io.EOF
