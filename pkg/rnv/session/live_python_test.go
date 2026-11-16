// SPDX-License-Identifier: LicenseRef-Reticulum
package session_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv/proto"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv/session"
)

// TestLivePythonSharedInstanceStillStream runs two Go RNV peers as clients of
// an isolated Python Reticulum shared-instance node (transport hub).
func TestLivePythonSharedInstanceStillStream(t *testing.T) {
	if testing.Short() && os.Getenv("REQUIRE_RNV_PYTHON") != "1" {
		t.Skip("live python shared-instance rnv skipped in -short")
	}
	py := pythonRNS(t)
	port := freeTCP(t)
	control := freeTCP(t)
	cfgDir := t.TempDir()
	script := filepath.Join(repoRoot(t), "testdata", "rnv", "rns_shared.py")
	cmd := exec.Command(py, script, "--configdir", cfgDir, "--port", fmt.Sprintf("%d", port), "--control-port", fmt.Sprintf("%d", control)) // #nosec G204 -- test harness
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
	waitPythonReady(t, stdout, 15*time.Second)

	trA, idA := startGoLocalClient(t, port)
	trB, idB := startGoLocalClient(t, port)

	stillCh := make(chan []byte, 1)
	videoCh := make(chan []byte, 4)
	cfgB := session.SafeConfig()
	cfgB.Handlers = session.Handlers{
		OnStill: func(_ *session.Conn, _ proto.StillMeta, data []byte) {
			stillCh <- append([]byte(nil), data...)
		},
		OnVideo: func(_ *session.Conn, fr proto.Frame) {
			videoCh <- append([]byte(nil), fr.Payload...)
		},
	}
	epB, err := session.Bind(trB, idB, cfgB)
	if err != nil {
		t.Fatal(err)
	}
	for range 6 {
		_ = epB.Announce()
		time.Sleep(80 * time.Millisecond)
	}
	identity.Remember(nil, epB.Hash(), idB.GetPublicKey(), nil)

	epA, err := session.Bind(trA, idA, session.SafeConfig())
	if err != nil {
		t.Fatal(err)
	}
	_ = epA.Announce()
	waitPath(t, trA, epB.Hash(), 20*time.Second)

	conn, err := epA.Dial(epB.Hash())
	if err != nil {
		t.Fatalf("dial via python shared instance: %v", err)
	}
	defer conn.Close()

	jpegBytes := tinyJPEG(t)
	if err := conn.SendStill(context.Background(), jpegBytes, proto.StillMeta{}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-stillCh:
		if !bytes.Equal(got, jpegBytes) {
			t.Fatalf("still mismatch")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("still timeout over python shared instance")
	}

	sc, err := conn.OpenStream(context.Background(), proto.StreamOffer{
		Profile: proto.ProfileMedium,
		Tracks:  proto.TrackVideo,
		Video:   proto.CodecJPEG,
	})
	if err != nil {
		t.Fatal(err)
	}
	frame := jpegBytes
	if len(frame) > proto.MaxStreamFrameBytes {
		frame = frame[:proto.MaxStreamFrameBytes]
	}
	if err := sc.SendVideo(frame); err != nil {
		t.Fatal(err)
	}
	select {
	case <-videoCh:
	case <-time.After(10 * time.Second):
		t.Fatal("video timeout over python shared instance")
	}
}

func startGoLocalClient(t *testing.T, port int) (*transport.Transport, *identity.Identity) {
	t.Helper()
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	cfg.EnableTransport = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	cfg.ConfigPath = t.TempDir() + "/config"
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
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if iface.IsOnline() {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !iface.IsOnline() {
		t.Fatalf("local client offline on port %d", port)
	}
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = iface.Stop() })
	return tr, id
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

func pythonRNS(t *testing.T) string {
	t.Helper()
	candidates := []string{os.Getenv("RNS_PYTHON"), "python3", "python"}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		cmd := exec.Command(c, "-c", "import RNS") // #nosec G204
		if err := cmd.Run(); err == nil {
			return c
		}
	}
	if os.Getenv("REQUIRE_RNV_PYTHON") == "1" {
		t.Fatal("python RNS required (set RNS_PYTHON or install rns)")
	}
	t.Skip("python RNS not available")
	return ""
}

func waitPythonReady(t *testing.T, r io.Reader, timeout time.Duration) {
	t.Helper()
	sc := bufio.NewScanner(r)
	type readyEvent struct {
		Event string `json:"event"`
		Error string `json:"error"`
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !sc.Scan() {
			break
		}
		var ev readyEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Event == "ready" {
			return
		}
		if ev.Event == "error" {
			t.Fatalf("python shared instance: %s", ev.Error)
		}
	}
	t.Fatal("python shared instance not ready")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for range 8 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod not found")
	return ""
}
