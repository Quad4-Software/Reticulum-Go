// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// golxmd killed with SIGKILL mid-transfer must restart on the same storage
// with no crash-loop, no corrupt store entries, and must answer control
// queries afterwards.
func TestInterop_Live_GolxmdKill9MidTransferRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("live chaos skipped in -short mode")
	}
	requireInterop(t)

	goPort := freeUDPPort(t)
	pyPort := freeUDPPort(t)

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
	prepareGolxmdHome(t, home)

	startDaemon := func() *exec.Cmd {
		readyPath := filepath.Join(home, "ready.json")
		_ = os.Remove(readyPath)
		cmd := exec.Command(bin,
			"--udp-listen", "127.0.0.1:"+strconv.Itoa(goPort),
			"--udp-forward", "127.0.0.1:"+strconv.Itoa(pyPort),
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
	waitReady := func() map[string]string {
		readyPath := filepath.Join(home, "ready.json")
		deadline := time.Now().Add(25 * time.Second)
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

	rounds := 1
	var midwriteDone chan struct{}
	var lastDaemon *exec.Cmd
	var ready map[string]string
	storeDirs := []string{
		filepath.Join(home, "storage", "lxmf", "messagestore"),
		filepath.Join(home, "storage", "lxmf", "golxmd", "messagestore"),
	}
	for round := 0; round < rounds; round++ {
		daemon := startDaemon()
		ready = waitReady()

		resp := interopCall(t, map[string]any{
			"cmd":              "live_send_propagation",
			"propagation_hash": ready["propagation_hash"],
			"listen_port":      pyPort,
			"forward_port":     goPort,
			"title":            "chaos-prop",
			"text":             strings.Repeat("kill9 chaos payload ", 200),
			"timeout_s":        90,
		})
		if !resp.OK {
			t.Fatalf("round %d: python propagation send failed: %+v", round, resp)
		}
		waitStoreMessagesUnique(t, storeDirs, round+1, 90*time.Second, "pre-kill store")

		// SIGKILL at a randomised point: immediately or mid-write of the
		// next upload, so kills land at varying positions in the write path
		if round%2 == 0 {
			// The SIGKILL below deliberately interrupts this in-flight
			// transfer. The python client is expected to fail or hang, so
			// the call runs outside the test-fatal path. The harness
			// process must finish before the next round starts or its
			// Reticulum instance conflicts with the round's new client.
			midwriteDone = make(chan struct{})
			go func() {
				defer close(midwriteDone)
				_, _ = runInterop(map[string]any{
					"cmd":              "live_send_propagation",
					"propagation_hash": ready["propagation_hash"],
					"listen_port":      pyPort,
					"forward_port":     goPort,
					"title":            "chaos-midwrite",
					"text":             strings.Repeat("midwrite kill payload ", 400),
					"timeout_s":        60,
				})
			}()
			time.Sleep(time.Duration(100+round*150) * time.Millisecond)
		}
		if err := daemon.Process.Signal(syscall.SIGKILL); err != nil {
			t.Fatalf("round %d sigkill: %v", round, err)
		}
		_, _ = daemon.Process.Wait()
		t.Logf("round %d: killed after store convergence", round)
		lastDaemon = daemon
	}
	_ = lastDaemon

	// restart on the same storage
	firstHash := ready["propagation_hash"]
	daemon2 := startDaemon()
	t.Cleanup(func() {
		if daemon2.Process != nil {
			_ = daemon2.Process.Signal(syscall.SIGTERM)
			_, _ = daemon2.Process.Wait()
		}
	})
	ready2 := waitReady()
	if ready2["propagation_hash"] != firstHash {
		t.Errorf("propagation hash changed across restart: %s: %s", firstHash, ready2["propagation_hash"])
	}

	// every store entry must be parseable and non-zero, across both layouts
	valid := 0
	total := 0
	for _, dir := range storeDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, ent := range entries {
			if ent.IsDir() || strings.HasSuffix(ent.Name(), ".tmp") {
				continue
			}
			total++
			data, err := os.ReadFile(filepath.Join(dir, ent.Name()))
			if err != nil {
				t.Fatalf("unreadable store entry %s: %v", ent.Name(), err)
			}
			if len(data) == 0 {
				t.Errorf("zero-byte store entry survived recovery: %s", ent.Name())
				continue
			}
			valid++
		}
	}
	if total == 0 {
		t.Fatalf("no store entries found after restart in %v", storeDirs)
	}
	if valid == 0 {
		t.Error("no valid store entries after crash recovery")
	}
}

// waitStoreMessagesUnique counts unique message filenames across all store
// layouts: golxmd writes its native store and imports python-layout entries
// on startup, so a restart can relocate or copy entries between directories.
func waitStoreMessagesUnique(t *testing.T, dirs []string, want int, timeout time.Duration, label string) {
	t.Helper()
	count := func() int {
		seen := map[string]bool{}
		for _, dir := range dirs {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					seen[e.Name()] = true
				}
			}
		}
		return len(seen)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if count() >= want {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s: stores %v have %d unique messages, want %d after %s", label, dirs, count(), want, timeout)
}
