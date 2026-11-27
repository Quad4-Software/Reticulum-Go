// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/hex"
	"encoding/json"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

func TestInterop_Live_PythonDeliveryGoGolxmd(t *testing.T) {
	if testing.Short() {
		t.Skip("live golxmd interop skipped in -short mode")
	}
	requireInterop(t)

	const (
		goPort = 43200
		pyPort = 43201
	)

	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	ready, stderr := startGolxmdDaemon(t, root, home, golxmdDaemonOpts{
		listenPort:      goPort,
		forwardPort:     pyPort,
		announceAtStart: true,
	})

	if ready["delivery_hash"] == "" {
		t.Fatalf("missing delivery_hash in ready: %+v", ready)
	}

	wantText := "hello from python to golxmd"
	resp := interopCall(t, map[string]any{
		"cmd":          "live_send_delivery",
		"dest_hash":    ready["delivery_hash"],
		"listen_port":  pyPort,
		"forward_port": goPort,
		"title":        "interop",
		"text":         wantText,
		"timeout_s":    45,
	})
	if !resp.OK {
		t.Fatalf("python send failed: %+v log=%s", resp, stderr.String())
	}

	msgDir := filepath.Join(home, "storage", "messages")
	found := false
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(msgDir)
		if err == nil {
			for _, ent := range entries {
				if ent.IsDir() {
					continue
				}
				_, msg, err := ReadFromFile(filepath.Join(msgDir, ent.Name()), RecallSource)
				if err != nil || msg == nil {
					continue
				}
				if msg.ContentString() == wantText {
					found = true
					break
				}
			}
		}
		if found {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !found {
		t.Fatalf("golxmd did not persist message log=%s", stderr.String())
	}
}

func TestInterop_Live_GoPropagationGoGolxmd(t *testing.T) {
	if testing.Short() {
		t.Skip("live golxmd propagation interop skipped in -short mode")
	}
	requireInterop(t)

	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	bin := filepath.Join(home, "golxmd")
	build := exec.Command("go", "build", "-mod=vendor", "-o", bin, "./cmd/golxmd")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build golxmd: %v\n%s", err, out)
	}

	env := append(os.Environ(), "GOLXMD_HOME="+home)
	prepareGolxmdHome(t, home)

	readyPath := filepath.Join(home, "ready.json")
	daemon := exec.Command(bin,
		"--propagation-node",
		"--udp-listen", "127.0.0.1:43210",
		"--udp-forward", "127.0.0.1:43211",
		"--ready-file", readyPath,
	)
	daemon.Env = env
	stderr := &strings.Builder{}
	daemon.Stderr = stderr
	daemon.Stdout = stderr
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if daemon.Process != nil {
			_ = daemon.Process.Signal(syscall.SIGTERM)
			_, _ = daemon.Process.Wait()
		}
	}()

	var ready map[string]string
	deadline := time.Now().Add(25 * time.Second)
	for {
		raw, err := os.ReadFile(filepath.Clean(readyPath)) // #nosec G304 -- test temp ready file
		if err == nil && len(raw) > 0 {
			var out map[string]string
			if json.Unmarshal(raw, &out) == nil && out["propagation_hash"] != "" {
				ready = out
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout golxmd ready\n%s", stderr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}

	if ready["propagation_hash"] == "" {
		t.Fatalf("missing propagation_hash: %+v", ready)
	}

	pnHash, err := hex.DecodeString(ready["propagation_hash"])
	if err != nil || len(pnHash) != DestinationLength {
		t.Fatalf("propagation_hash=%q err=%v", ready["propagation_hash"], err)
	}

	cfg := testconfig.New()
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()
	iface := startLXMFUDP(t, "PN-client", "127.0.0.1:43211", "127.0.0.1:43210", tr)
	defer func() { _ = iface.Stop() }()

	id1 := mustNewIdentity(t)
	id2 := mustNewIdentity(t)
	dest1, err := NewDeliveryDestination(id1, tr)
	if err != nil {
		t.Fatal(err)
	}
	dest2, err := NewDeliveryDestination(id2, tr)
	if err != nil {
		t.Fatal(err)
	}
	h1 := dest1.GetHash()
	h2 := append([]byte(nil), pnHash...)
	recipientHash := dest2.GetHash()
	identity.Remember(nil, h2, nil, nil)
	identity.Remember(nil, recipientHash, id2.GetPublicKey(), nil)
	identity.Remember(nil, h1, id1.GetPublicKey(), nil)

	m1 := NewMessenger(tr, dest1)

	_ = dest1.Announce(false, nil, nil)
	pathDeadline := time.Now().Add(pathEstablishWait)
	for !tr.HasPath(h2) {
		if time.Now().After(pathDeadline) {
			t.Fatalf("no path to golxmd PN log=%s", stderr.String())
		}
		_ = dest1.Announce(false, nil, nil)
		time.Sleep(40 * time.Millisecond)
	}

	if remote, err := identity.Recall(h2); err == nil && remote != nil {
		identity.Remember(nil, h2, remote.GetPublicKey(), nil)
	}

	msg, err := NewMessage(recipientHash, h1, []byte("pn"), []byte("propagated-to-golxmd"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := msg.Pack(id1); err != nil {
		t.Fatal(err)
	}

	if err := m1.SendPropagated(msg, h2, PropagationStampCostMin); err != nil {
		t.Fatalf("SendPropagated: %v log=%s", err, stderr.String())
	}

	storeDir := filepath.Join(home, "storage", "lxmf", "messagestore")
	found := false
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(storeDir)
		if err == nil && len(entries) > 0 {
			for _, ent := range entries {
				if !ent.IsDir() && !strings.HasSuffix(ent.Name(), ".tmp") {
					found = true
					break
				}
			}
		}
		if found {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !found {
		t.Fatalf("golxmd propagation store empty log=%s", stderr.String())
	}
}

func TestInterop_Live_PythonPropagationGoGolxmd(t *testing.T) {
	if testing.Short() {
		t.Skip("live golxmd python propagation interop skipped in -short mode")
	}
	requireInterop(t)

	const (
		goPort = 43212
		pyPort = 43213
	)

	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	ready, stderr := startGolxmdDaemon(t, root, home, golxmdDaemonOpts{
		listenPort:      goPort,
		forwardPort:     pyPort,
		propagationNode: true,
		announceAtStart: true,
	})

	if ready["propagation_hash"] == "" {
		t.Fatalf("missing propagation_hash: %+v", ready)
	}

	wantText := "hello from python propagation to golxmd"
	resp := interopCall(t, map[string]any{
		"cmd":              "live_send_propagation",
		"propagation_hash": ready["propagation_hash"],
		"listen_port":      pyPort,
		"forward_port":     goPort,
		"title":            "interop-prop",
		"text":             wantText,
		"timeout_s":        120,
	})
	if !resp.OK {
		t.Fatalf("python propagation send failed: %+v log=%s", resp, stderr.String())
	}

	storeDir := filepath.Join(home, "storage", "lxmf", "messagestore")
	found := false
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(storeDir)
		if err == nil && len(entries) > 0 {
			for _, ent := range entries {
				if !ent.IsDir() && !strings.HasSuffix(ent.Name(), ".tmp") {
					found = true
					break
				}
			}
		}
		if found {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !found {
		t.Fatalf("golxmd propagation store empty after python upload log=%s", stderr.String())
	}
}

func TestInterop_Live_GoDeliveryGolxmd(t *testing.T) {
	if testing.Short() {
		t.Skip("live golxmd delivery interop skipped in -short mode")
	}
	requireInterop(t)

	const (
		goPort = 43220
		pyPort = 43221
	)

	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	ready, stderr := startGolxmdDaemon(t, root, home, golxmdDaemonOpts{
		listenPort:      goPort,
		forwardPort:     pyPort,
		announceAtStart: true,
	})

	cfg := testconfig.New()
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()
	iface := startLXMFUDP(t, "Go-client", "127.0.0.1:"+strconv.Itoa(pyPort), "127.0.0.1:"+strconv.Itoa(goPort), tr)
	defer func() { _ = iface.Stop() }()

	id := mustNewIdentity(t)
	messenger, err := NewDeliveryMessenger(id, tr)
	if err != nil {
		t.Fatal(err)
	}
	destHash, err := hex.DecodeString(ready["delivery_hash"])
	if err != nil || len(destHash) != DestinationLength {
		t.Fatalf("delivery_hash=%q err=%v", ready["delivery_hash"], err)
	}

	deadline := time.Now().Add(25 * time.Second)
	for !tr.HasPath(destHash) {
		if time.Now().After(deadline) {
			t.Fatalf("no path to golxmd delivery log=%s", stderr.String())
		}
		_ = messenger.Destination().Announce(false, nil, nil)
		time.Sleep(50 * time.Millisecond)
	}

	wantText := "hello from go to golxmd"
	if _, err := messenger.SendText(destHash, "interop", wantText); err != nil {
		t.Fatalf("send: %v log=%s", err, stderr.String())
	}

	msgDir := filepath.Join(home, "storage", "messages")
	found := false
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(msgDir)
		if err == nil {
			for _, ent := range entries {
				if ent.IsDir() {
					continue
				}
				_, msg, err := ReadFromFile(filepath.Join(msgDir, ent.Name()), RecallSource)
				if err != nil || msg == nil {
					continue
				}
				if msg.ContentString() == wantText {
					found = true
					break
				}
			}
		}
		if found {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !found {
		t.Fatalf("golxmd did not persist go delivery log=%s", stderr.String())
	}
}

func prepareGolxmdHome(t *testing.T, home string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "storage", "messages"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config"), []byte(DefaultConfigText), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := id.ToFile(filepath.Join(home, "identity")); err != nil {
		t.Fatal(err)
	}
}

type golxmdDaemonOpts struct {
	listenPort      int
	forwardPort     int
	propagationNode bool
	announceAtStart bool
}

func startGolxmdDaemon(t *testing.T, root, home string, opts golxmdDaemonOpts) (map[string]string, *strings.Builder) {
	t.Helper()

	bin := filepath.Join(home, "golxmd")
	build := exec.Command("go", "build", "-mod=vendor", "-o", bin, "./cmd/golxmd")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build golxmd: %v\n%s", err, out)
	}

	env := append(os.Environ(), "GOLXMD_HOME="+home)
	prepareGolxmdHome(t, home)

	if opts.announceAtStart {
		cfgPath := filepath.Join(home, "config")
		raw, err := os.ReadFile(filepath.Clean(cfgPath)) // #nosec G304 -- test temp config
		if err != nil {
			t.Fatal(err)
		}
		cfgText := strings.Replace(string(raw), "announce_at_start = no", "announce_at_start = yes", 1)
		if err := os.WriteFile(cfgPath, []byte(cfgText), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	readyPath := filepath.Join(home, "ready.json")
	args := []string{
		"--udp-listen", "127.0.0.1:" + strconv.Itoa(opts.listenPort),
		"--udp-forward", "127.0.0.1:" + strconv.Itoa(opts.forwardPort),
		"--ready-file", readyPath,
		"--service",
	}
	if opts.propagationNode {
		args = append(args, "--propagation-node")
	}
	daemon := exec.Command(bin, args...)
	daemon.Env = env
	stderr := &strings.Builder{}
	daemon.Stderr = stderr
	daemon.Stdout = stderr
	if err := daemon.Start(); err != nil {
		t.Fatalf("start golxmd: %v", err)
	}
	t.Cleanup(func() {
		if daemon.Process != nil {
			_ = daemon.Process.Signal(syscall.SIGTERM)
			_, _ = daemon.Process.Wait()
		}
	})

	var ready map[string]string
	deadline := time.Now().Add(25 * time.Second)
	for {
		raw, err := os.ReadFile(filepath.Clean(readyPath)) // #nosec G304 -- test temp ready file
		if err == nil && len(raw) > 0 {
			var out map[string]string
			if json.Unmarshal(raw, &out) == nil {
				if opts.propagationNode {
					if out["propagation_hash"] != "" {
						ready = out
						break
					}
				} else if out["delivery_hash"] != "" {
					ready = out
					break
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for golxmd ready\n%s", stderr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ready, stderr
}
