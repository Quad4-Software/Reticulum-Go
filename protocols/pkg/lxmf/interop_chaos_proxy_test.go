// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/faultproxy"
)

func chaosSeed(t *testing.T) int64 {
	t.Helper()
	seed := int64(20240101)
	if v := os.Getenv("LXMF_CHAOS_SEED"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed = n
		}
	}
	t.Logf("chaos seed: %d (replay with LXMF_CHAOS_SEED=%d)", seed, seed)
	return seed
}

func writeGolxmdTCPClientRNSConfig(t *testing.T, dir string, targetPort int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "storage", "identities"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`[reticulum]
  enable_transport = Yes
  share_instance = No
  shared_instance_port = 0
  instance_name = golxmd-chaos-%d

[logging]
  loglevel = 4

[interfaces]

  [[Hub]]
    type = TCPClientInterface
    interface_enabled = True
    target_host = 127.0.0.1
    target_port = %d
`, targetPort, targetPort)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

type chaosDaemon struct {
	cmd  *exec.Cmd
	home string
}

func startChaosGolxmd(t *testing.T, bin, home, rnsDir string) *exec.Cmd {
	t.Helper()
	readyPath := filepath.Join(home, "ready.json")
	_ = os.Remove(readyPath)
	cmd := exec.Command(bin,
		"--rnsconfig", rnsDir,
		"--ready-file", readyPath,
		"--service",
		"--propagation-node",
	)
	cmd.Env = append(os.Environ(), "GOLXMD_HOME="+home)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start golxmd: %v", err)
	}
	return cmd
}

func waitChaosReady(t *testing.T, home string) map[string]string {
	t.Helper()
	readyPath := filepath.Join(home, "ready.json")
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(filepath.Clean(readyPath)) // #nosec G304 -- test temp
		if err == nil && len(raw) > 0 {
			var out map[string]string
			if json.Unmarshal(raw, &out) == nil && out["propagation_hash"] != "" {
				return out
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for golxmd ready, log=%s", readFileTail(filepath.Join(home, "logfile"), 20000))
	return nil
}

func buildChaosGolxmd(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "golxmd")
	build := exec.Command("go", "build", "-mod=vendor", "-o", bin, "./cmd/golxmd")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build golxmd: %v\n%s", err, out)
	}
	return bin
}

func stopChaosDaemon(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_, _ = cmd.Process.Wait()
	}
}

// Propagation upload/fetch convergence through the fault-injecting proxy
// under moderate frame loss, duplication, reordering, and delay.
func TestInterop_Live_ChaosProxyConvergence(t *testing.T) {
	if testing.Short() {
		t.Skip("live chaos skipped in -short mode")
	}
	requireInterop(t)
	seed := chaosSeed(t)

	hubPort := freeTCPPort(t)
	hubDir := t.TempDir()
	writeHubRNSConfig(t, hubDir, hubPort)
	hub := startPythonRnsd(t, hubDir)

	faults := faultproxy.Faults{Drop: 0.02, Duplicate: 0.02, Reorder: 0.02, DelayMs: 5, JitterMs: 10}
	if os.Getenv("CHAOS_TRANSPARENT") == "1" {
		faults = faultproxy.Faults{}
	}
	if v := os.Getenv("CHAOS_FAULTS"); v != "" {
		// format: drop,dup,reorder,delayms,jitterms
		var d, du, r, dm, jm float64
		if n, err := fmt.Sscanf(v, "%g,%g,%g,%g,%g", &d, &du, &r, &dm, &jm); err == nil && n == 5 {
			faults = faultproxy.Faults{Drop: d, Duplicate: du, Reorder: r, DelayMs: int(dm), JitterMs: int(jm)}
		}
	}
	t.Logf("faults: %s", faults)
	proxy, err := faultproxy.New(fmt.Sprintf("127.0.0.1:%d", hubPort), seed, faults)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proxy.Close() }()
	proxyPort, _ := strconv.Atoi(strings.Split(proxy.Addr, ":")[1])

	bin := buildChaosGolxmd(t)
	goHome := t.TempDir()
	prepareGolxmdHome(t, goHome)
	rnsDir := t.TempDir()
	writeGolxmdTCPClientRNSConfig(t, rnsDir, proxyPort)

	baseG := runtime.NumGoroutine()
	daemon := startChaosGolxmd(t, bin, goHome, rnsDir)
	defer stopChaosDaemon(daemon)
	ready := waitChaosReady(t, goHome)

	// python client connects directly to the hub (unfaulted side)
	convergeDeadline := time.Now().Add(150 * time.Second)
	resp := interopCall(t, map[string]any{
		"cmd":              "live_send_propagation",
		"hub_port":         hubPort,
		"propagation_hash": ready["propagation_hash"],
		"title":            "chaos-converge",
		"text":             "convergence under fault injection",
		"timeout_s":        120,
	})
	if !resp.OK {
		t.Fatalf("python propagation send through chaos topology failed: %+v", resp)
	}

	storeDir := filepath.Join(goHome, "storage", "lxmf", "messagestore")
	waitStoreMessages(t, storeDir, 1, time.Until(convergeDeadline), "chaos store convergence")

	s := proxy.Snapshot()
	t.Logf("proxy stats: %+v seed=%d", s, seed)
	if s.FramesIn == 0 {
		t.Error("proxy saw no frames — golxmd is not routing through it")
	}
	if got := runtime.NumGoroutine(); got > baseG+200 {
		t.Errorf("test-side goroutines unbounded: base=%d after=%d", baseG, got)
	}
	_ = hub
}

// Repeated connection resets during transfer: the node must survive link
// flaps, and a retry after the flapping stops must converge.
func TestInterop_Live_ChaosLinkFlaps(t *testing.T) {
	if testing.Short() {
		t.Skip("live chaos skipped in -short mode")
	}
	requireInterop(t)
	seed := chaosSeed(t)

	hubPort := freeTCPPort(t)
	hubDir := t.TempDir()
	writeHubRNSConfig(t, hubDir, hubPort)
	startPythonRnsd(t, hubDir)

	proxy, err := faultproxy.New(fmt.Sprintf("127.0.0.1:%d", hubPort), seed, faultproxy.Faults{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proxy.Close() }()
	proxyPort, _ := strconv.Atoi(strings.Split(proxy.Addr, ":")[1])

	bin := buildChaosGolxmd(t)
	goHome := t.TempDir()
	prepareGolxmdHome(t, goHome)
	rnsDir := t.TempDir()
	writeGolxmdTCPClientRNSConfig(t, rnsDir, proxyPort)

	daemon := startChaosGolxmd(t, bin, goHome, rnsDir)
	defer stopChaosDaemon(daemon)
	ready := waitChaosReady(t, goHome)

	// flap the connection repeatedly while the client is active
	flapDone := make(chan struct{})
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(1500 * time.Millisecond)
			proxy.Flap()
		}
		close(flapDone)
	}()

	resp := interopCall(t, map[string]any{
		"cmd":              "live_send_propagation",
		"hub_port":         hubPort,
		"propagation_hash": ready["propagation_hash"],
		"title":            "chaos-flap",
		"text":             "convergence across link flaps",
		"timeout_s":        100,
	})
	<-flapDone
	t.Logf("send during flaps ok=%v resp=%+v", resp.OK, resp)

	// after flapping stops, convergence must succeed within bounded time
	deadline := time.Now().Add(120 * time.Second)
	converged := false
	for time.Now().Before(deadline) {
		resp = interopCall(t, map[string]any{
			"cmd":              "live_send_propagation",
			"hub_port":         hubPort,
			"propagation_hash": ready["propagation_hash"],
			"title":            "chaos-flap-after",
			"text":             "post-flap convergence",
			"timeout_s":        30,
		})
		if resp.OK {
			converged = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !converged {
		t.Fatalf("no convergence after link flaps, golxmd log=%s", readFileTail(filepath.Join(goHome, "logfile"), 20000))
	}

	storeDir := filepath.Join(goHome, "storage", "lxmf", "messagestore")
	waitStoreMessages(t, storeDir, 1, 60*time.Second, "post-flap store")

	entries, err := os.ReadDir(storeDir)
	if err == nil {
		for _, ent := range entries {
			if ent.IsDir() || strings.HasSuffix(ent.Name(), ".tmp") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(storeDir, ent.Name()))
			if err != nil || len(data) == 0 {
				t.Errorf("corrupt store entry after flaps: %s", ent.Name())
			}
		}
	}
}
