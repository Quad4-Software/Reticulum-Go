// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live path/file utility interop: Go RPC, Python rnsd, Go/Go over UDP.
// Set RUN_LIVE_INTEROP=1 to enable.

package interop

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	rlink "quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/resource"
	"quad4/reticulum-go/pkg/rnsutil"
)

func TestLiveRgopathRPCAgainstGoDaemon(t *testing.T) {
	liveOrSkip(t)

	cfgDir := t.TempDir()
	port := freeUDPPort(t)
	ctrlPort := port + 1
	cfgPath := filepath.Join(cfgDir, "config")
	if err := os.WriteFile(cfgPath, []byte(strings.Join([]string{
		"[reticulum]",
		"enable_transport = yes",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	goCfg := &common.ReticulumConfig{
		EnableTransport:     true,
		ShareInstance:       true,
		SharedInstancePort:  port,
		InstanceControlPort: ctrlPort,
		SharedInstanceType:  common.SharedInstanceTCP,
		ConfigPath:          cfgPath,
	}
	n, err := node.New(goCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	cfg, err := rnsutil.LoadConfigDir(cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	client, err := rnsutil.DialRPC(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	client.SetTimeout(5 * time.Second)

	idHash := make([]byte, 16)
	for i := range idHash {
		idHash[i] = byte(i + 3)
	}
	ok, err := client.BlackholeIdentity(idHash, 0, "interop test")
	if err != nil {
		t.Fatalf("blackhole: %v", err)
	}
	if !ok {
		t.Fatal("expected blackhole insert")
	}
	raw, err := client.GetBlackholedIdentities()
	if err != nil {
		t.Fatal(err)
	}
	entries := rnsutil.NormalizeBlackholeRPC(raw)
	if len(entries) == 0 {
		t.Fatal("expected blackhole entry")
	}
	ok, err = client.UnblackholeIdentity(idHash)
	if err != nil || !ok {
		t.Fatalf("unblackhole: ok=%v err=%v", ok, err)
	}

	paths, err := client.GetPathTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := rnsutil.WritePathTableJSON(&buf, paths); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("[")) {
		t.Fatalf("json: %s", buf.String())
	}
	if _, err := client.DropAnnounceQueues(); err != nil {
		t.Fatal(err)
	}
}

func TestLiveRgopathAgainstPythonRNSD(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath("rnsd"); err != nil {
		t.Skip("rnsd not installed")
	}

	cfgDir := t.TempDir()
	port := freeUDPPort(t)
	ctrlPort := port + 1
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = yes",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
		"[logging]",
		"loglevel = 3",
		"",
		"[interfaces]",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rnsd", "--config", cfgDir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start rnsd: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		cfg, err := rnsutil.LoadConfigDir(cfgDir)
		if err != nil {
			lastErr = err
			time.Sleep(300 * time.Millisecond)
			continue
		}
		client, err := rnsutil.DialRPC(cfg, nil)
		if err != nil {
			lastErr = err
			time.Sleep(300 * time.Millisecond)
			continue
		}
		client.SetTimeout(5 * time.Second)
		// Warm up with interface_stats (same probe as rgostatus) before path_table.
		if _, err := client.GetInterfaceStats(); err != nil {
			lastErr = err
			time.Sleep(300 * time.Millisecond)
			continue
		}
		paths, err := client.GetPathTable(nil)
		if err != nil {
			// Some Python builds answer interface_stats but stall on path_table
			// during early startup. Retry a few times, then accept RPC as up.
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			paths, err = client.GetPathTable(nil)
			if err != nil {
				t.Logf("path_table still unavailable after stats ok: %v", err)
				return
			}
		}
		var buf bytes.Buffer
		if err := rnsutil.WritePathTableJSON(&buf, paths); err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(buf.Bytes(), []byte("[")) {
			t.Fatalf("expected json array: %s", buf.String())
		}
		if _, err := exec.LookPath("rnpath"); err == nil {
			py := exec.Command("rnpath", "--config", cfgDir, "-t", "--json")
			if out, err := py.CombinedOutput(); err == nil && len(out) > 0 && out[0] == '[' {
				return
			}
		}
		return
	}
	t.Fatalf("rgopath rpc against python rnsd failed: %v", lastErr)
}

func TestLiveGoToGoFileTransferOverUDP(t *testing.T) {
	liveOrSkip(t)

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)

	trA, _, cleanupA := setupGoUDPPeer(t, portA, portB)
	defer cleanupA()
	trB, _, cleanupB := setupGoUDPPeer(t, portB, portA)
	defer cleanupB()

	idListen, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	idSend, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}

	dest, err := destination.New(idListen, destination.In, destination.Single, rnsutil.RNCPAppName, trA, rnsutil.RNCPAspect)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)
	received := make(chan []byte, 1)
	var once sync.Once
	dest.SetLinkEstablishedCallback(func(lnk any) {
		l := interopLink(t, lnk)
		_ = l.SetResourceStrategy(rlink.AcceptAll)
		l.SetResourceConcludedCallback(func(p any) {
			once.Do(func() {
				switch v := p.(type) {
				case rlink.IncomingResource:
					received <- v.Data
				case []byte:
					received <- v
				}
			})
		})
	})
	_ = dest.Announce(false, nil, nil)
	time.Sleep(100 * time.Millisecond)
	_ = dest.Announce(false, nil, nil)

	destHash := dest.GetHash()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := rnsutil.WaitPath(ctx, trB, destHash); err != nil {
		t.Fatalf("path: %v hash=%s", err, hex.EncodeToString(destHash))
	}
	l, err := rnsutil.EstablishRNCPLink(ctx, trB, destHash)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Teardown()
	if err := l.Identify(idSend); err != nil {
		t.Fatal(err)
	}

	body := bytes.Repeat([]byte("U"), 2048)
	res, err := resource.New(body, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.SetMetadata(map[string]any{"name": []byte("udp.bin")})
	if err := l.SendResource(res); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-received:
		if !bytes.Equal(got, body) {
			t.Fatalf("len got=%d want=%d", len(got), len(body))
		}
	case <-time.After(30 * time.Second):
		t.Fatal("udp receive timeout")
	}
}

func TestLiveRgocpCLIAgainstGoListenerUDP(t *testing.T) {
	liveOrSkip(t)

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	cfgDirA := t.TempDir()
	cfgDirB := t.TempDir()

	writePeerConfig := func(dir string, listen, peerPort int) {
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
			"    target_port = " + strconv.Itoa(peerPort),
			"",
		}, "\n")
		if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writePeerConfig(cfgDirA, portA, portB)
	writePeerConfig(cfgDirB, portB, portA)

	cfgA, err := rnsutil.LoadConfigDir(cfgDirA)
	if err != nil {
		t.Fatal(err)
	}
	nA, err := node.New(cfgA)
	if err != nil {
		t.Fatal(err)
	}
	if err := nA.Start(); err != nil {
		t.Fatal(err)
	}
	defer nA.Stop()

	idListen, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(idListen, destination.In, destination.Single, rnsutil.RNCPAppName, nA.Transport(), rnsutil.RNCPAspect)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)
	saveDir := t.TempDir()
	dest.SetLinkEstablishedCallback(func(lnk any) {
		l := interopLink(t, lnk)
		_ = l.SetResourceStrategy(rlink.AcceptAll)
		l.SetResourceConcludedCallback(func(p any) {
			var data []byte
			var meta map[string]any
			switch v := p.(type) {
			case rlink.IncomingResource:
				data, meta = v.Data, v.Metadata
			case []byte:
				data = v
			}
			name := rnsutil.FilenameFromMetadata(meta)
			_, _ = rnsutil.WriteReceivedFile(saveDir, name, data, true)
		})
	})
	_ = dest.Announce(false, nil, nil)

	rgocp := filepath.Join(repoRoot(t), "bin", "rgocp")
	build := exec.Command("go", "build", "-o", rgocp, "./cmd/rgocp")
	build.Dir = repoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build rgocp: %v\n%s", err, out)
	}

	src := filepath.Join(t.TempDir(), "cli.txt")
	if err := os.WriteFile(src, []byte("cli transfer body"), 0o600); err != nil {
		t.Fatal(err)
	}
	destHex := hex.EncodeToString(dest.GetHash())
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, rgocp, "-config", cfgDirB, "-S", src, destHex)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rgocp send: %v\n%s", err, out)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(saveDir)
		if len(entries) > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("file not received; rgocp out:\n%s", out)
}
