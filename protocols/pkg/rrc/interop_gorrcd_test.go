// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestInterop_Live_PythonClientGoDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("live python interop skipped in -short mode")
	}
	requireInterop(t)

	const (
		goPort = 42950
		pyPort = 42951
	)

	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	bin := filepath.Join(home, "gorrcd")
	build := exec.Command("go", "build", "-mod=vendor", "-o", bin, "./cmd/gorrcd")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build gorrcd: %v\n%s", err, out)
	}

	env := append(os.Environ(), "GORRCD_HOME="+home)
	first := exec.Command(bin)
	first.Env = env
	if out, err := first.CombinedOutput(); err != nil {
		t.Fatalf("gorrcd first run: %v\n%s", err, out)
	}

	readyPath := filepath.Join(home, "ready.json")
	daemon := exec.Command(bin,
		"--udp-listen", "127.0.0.1:42950",
		"--udp-forward", "127.0.0.1:42951",
		"--ready-file", readyPath,
		"--hub-name", "go-daemon",
		"--greeting", "welcome-daemon",
		"--announce-period", "1",
		"--log-level", "WARNING",
	)
	daemon.Env = env
	stderr := &strings.Builder{}
	daemon.Stderr = stderr
	daemon.Stdout = stderr
	if err := daemon.Start(); err != nil {
		t.Fatalf("start gorrcd: %v", err)
	}
	defer func() {
		if daemon.Process != nil {
			_ = daemon.Process.Signal(syscall.SIGTERM)
			_, _ = daemon.Process.Wait()
		}
	}()

	deadline := time.Now().Add(20 * time.Second)
	var ready map[string]string
	for {
		raw, err := os.ReadFile(filepath.Clean(readyPath)) // #nosec G304 -- test temp ready file
		if err == nil && len(raw) > 0 {
			var out map[string]string
			if json.Unmarshal(raw, &out) == nil && out["hub_hash"] != "" {
				ready = out
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for gorrcd ready file\n%s", stderr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	hubHash, err := hex.DecodeString(ready["hub_hash"])
	if err != nil || len(hubHash) != IdentityLength {
		t.Fatalf("hub_hash=%q err=%v log=%s", ready["hub_hash"], err, stderr.String())
	}
	if ready["name"] != "go-daemon" {
		t.Fatalf("ready name=%q", ready["name"])
	}

	wantText := "hello from python rrc client"
	resp := interopCall(t, map[string]any{
		"cmd":          "client_session",
		"hub_hash":     hex.EncodeToString(hubHash),
		"listen_port":  pyPort,
		"forward_port": goPort,
		"room":         "#lobby",
		"text":         wantText,
		"nick":         "py-daemon",
		"timeout_s":    45,
		"steps":        []string{"join", "msg", "list", "who", "unrecognized", "ping", "action", "part"},
	})
	if !resp.Joined {
		t.Fatalf("not joined: %+v log=%s", resp, stderr.String())
	}
	if resp.HubName != "go-daemon" {
		t.Fatalf("hub_name=%q", resp.HubName)
	}
	if resp.Text != wantText {
		t.Fatalf("text=%q", resp.Text)
	}
	if !resp.MsgEcho {
		t.Fatalf("missing msg echo notices=%v errors=%v log=%s", resp.Notices, resp.Errors, stderr.String())
	}
	if !resp.Pong {
		t.Fatal("missing pong")
	}
	foundGreeting := false
	foundList := false
	foundWho := false
	for _, n := range resp.Notices {
		if strings.Contains(n, "welcome-daemon") {
			foundGreeting = true
		}
		if strings.Contains(strings.ToLower(n), "public rooms") || strings.Contains(n, "Registered") {
			foundList = true
		}
		if strings.Contains(n, "members in") {
			foundWho = true
		}
	}
	if !foundGreeting {
		t.Fatalf("missing greeting notices=%v", resp.Notices)
	}
	if !foundList {
		t.Fatalf("missing /list notice notices=%v", resp.Notices)
	}
	if !foundWho {
		t.Fatalf("missing /who notice notices=%v", resp.Notices)
	}
	foundUnrec := false
	for _, e := range resp.Errors {
		if strings.Contains(e, "unrecognized command") {
			foundUnrec = true
		}
	}
	if !foundUnrec {
		t.Fatalf("missing unrecognized command errors=%v", resp.Errors)
	}
}
