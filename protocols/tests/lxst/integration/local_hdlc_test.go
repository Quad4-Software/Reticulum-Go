// SPDX-License-Identifier: LicenseRef-Reticulum
package integration_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/lxsttest"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/rnsnode"
)

func TestStartLocalRegistersSharedInstanceName(t *testing.T) {
	port := freeTCP(t)
	hubCfg := isolatedTransportConfig(t)
	hubCfg.EnableTransport = true
	hub := transport.NewTransport(hubCfg)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	spawn := func(client *interfaces.LocalClientInterface) {
		client.In = true
		client.Out = true
		if err := hub.RegisterInterface(client.GetName(), client); err != nil {
			t.Errorf("register spawned local client: %v", err)
		}
	}
	srv, err := interfaces.NewLocalServerInterface(port, "", false, spawn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Stop() }()

	sess, err := rnsnode.Start(rnsnode.Options{Kind: rnsnode.KindLocal, LocalPort: port})
	if err != nil {
		t.Fatal(err)
	}
	ifaces := sess.Transport.GetInterfaces()
	if _, ok := ifaces["Local shared instance"]; !ok {
		names := make([]string, 0, len(ifaces))
		for n := range ifaces {
			names = append(names, n)
		}
		t.Fatalf("interface registered as %v, want Local shared instance", names)
	}
	if !sess.Transport.ConnectedToSharedInstance() {
		t.Fatal("expected shared-instance client flag")
	}
}

func TestGoSharedInstanceAnnounce(t *testing.T) {
	port := freeTCP(t)
	hubCfg := isolatedTransportConfig(t)
	hubCfg.EnableTransport = true
	hub := transport.NewTransport(hubCfg)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}

	spawn := func(client *interfaces.LocalClientInterface) {
		client.In = true
		client.Out = true
		name := client.GetName()
		if err := hub.RegisterInterface(name, client); err != nil {
			t.Errorf("register spawned local client: %v", err)
		}
	}
	srv, err := interfaces.NewLocalServerInterface(port, "", false, spawn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Stop() }()

	tA, idA := startGoLocal(t, port)
	tB, idB := startGoLocal(t, port)

	destA, err := destination.New(idA, destination.In, destination.Single, proto.AppName, tA, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	destA.AcceptsLinks(true)
	for range 8 {
		_ = destA.Announce(false, nil, nil)
		time.Sleep(80 * time.Millisecond)
	}

	hash := destA.GetHash()
	if err := waitPath(hub, hash, 3*time.Second); err != nil {
		t.Fatalf("shared instance hub missing path after HDLC announce: %v", err)
	}
	if err := waitPath(tB, hash, 3*time.Second); err != nil {
		t.Fatalf("shared instance client missing path after HDLC announce: %v", err)
	}
	if _, err := identity.Recall(hash); err != nil {
		t.Fatalf("client did not remember announced identity: %v", err)
	}
	_ = idB
}

func TestPythonSharedInstanceAnnounce(t *testing.T) {
	lxsttest.SkipLive(t)
	py := lxsttest.Python(t)
	port := freeTCP(t)
	cfgDir := t.TempDir()
	script := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "rns_shared.py")
	cmd := exec.Command(py, script, "--configdir", cfgDir, "--port", fmt.Sprintf("%d", port), "--control-port", fmt.Sprintf("%d", freeTCP(t)))
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	sc := bufio.NewScanner(stdout)
	deadline := time.Now().Add(10 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		if !sc.Scan() {
			break
		}
		var ev peerEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Event == "ready" {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatal("could not start an isolated python shared instance")
	}

	tr, id := startGoLocal(t, port)
	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, tr, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)
	if err := dest.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce over python shared instance HDLC: %v", err)
	}
}

func startGoLocal(t *testing.T, port int) (*transport.Transport, *identity.Identity) {
	t.Helper()
	cfg := isolatedTransportConfig(t)
	cfg.EnableTransport = false
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	iface, err := interfaces.NewLocalClientInterface(port, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	iface.In = true
	iface.Out = true
	if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatal(err)
	}
	if err := iface.Start(); err != nil {
		t.Fatal(err)
	}
	if !iface.IsOnline() {
		t.Fatalf("local client offline on port %d", port)
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	return tr, id
}

func isolatedTransportConfig(t *testing.T) *common.ReticulumConfig {
	t.Helper()
	cfg := testconfig.New()
	cfg.ShareInstance = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	cfg.ConfigPath = t.TempDir() + "/config"
	return cfg
}

func freeTCP(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}
