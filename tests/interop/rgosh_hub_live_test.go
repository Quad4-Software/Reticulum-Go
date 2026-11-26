// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type testProcess struct {
	cmd    *exec.Cmd
	stdout *lockedBuffer
	stderr *lockedBuffer
}

func buildHubTestBinaries(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	daemonBin := filepath.Join(dir, "reticulum-go")
	rgoshBin := filepath.Join(dir, "rgosh")
	pathBin := filepath.Join(dir, "rgopath")
	for _, build := range []struct {
		out string
		pkg string
	}{
		{daemonBin, "./cmd/reticulum-go"},
		{rgoshBin, "./cmd/rgosh"},
		{pathBin, "./cmd/rgopath"},
	} {
		cmd := exec.Command("go", "build", "-o", build.out, build.pkg)
		cmd.Dir = repoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", build.pkg, err, out)
		}
	}
	return daemonBin, rgoshBin, pathBin
}

func writeHubDaemonConfig(t *testing.T, dir string, port int, share bool) {
	t.Helper()
	writeHubConfig(t, dir, "yes", share, strings.Join([]string{
		"  [[hub]]",
		"    type = TCPServerInterface",
		"    enabled = yes",
		"    listen_ip = 127.0.0.1",
		"    listen_port = " + strconv.Itoa(port),
	}, "\n"))
}

func writeHubClientConfig(t *testing.T, dir string, port int, share bool) {
	t.Helper()
	writeHubConfig(t, dir, "no", share, strings.Join([]string{
		"  [[hub]]",
		"    type = TCPClientInterface",
		"    enabled = yes",
		"    target_host = 127.0.0.1",
		"    target_port = " + strconv.Itoa(port),
	}, "\n"))
}

func writeHubConfig(t *testing.T, dir, transport string, share bool, iface string) {
	t.Helper()
	shareValue := "no"
	if share {
		shareValue = "yes"
	}
	sharedPort := freeTCPPort(t)
	controlPort := freeTCPPort(t)
	cfg := strings.Join([]string{
		"[reticulum]",
		"enable_transport = " + transport,
		"share_instance = " + shareValue,
		"shared_instance_type = tcp",
		"shared_instance_port = " + strconv.Itoa(sharedPort),
		"instance_control_port = " + strconv.Itoa(controlPort),
		"loglevel = 4",
		"",
		"[interfaces]",
		iface,
		"",
		"[stamps]",
		"destination_value = 8",
		"destination_cost = 8",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

func startTestProcess(t *testing.T, ctx context.Context, name string, argv ...string) *testProcess {
	t.Helper()
	if len(argv) == 0 {
		t.Fatal("empty process argv")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	p := &testProcess{cmd: cmd, stdout: &lockedBuffer{}, stderr: &lockedBuffer{}}
	cmd.Stdout = p.stdout
	cmd.Stderr = p.stderr
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	t.Cleanup(func() { stopTestProcess(p) })
	return p
}

func stopTestProcess(p *testProcess) {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	if p.cmd.ProcessState == nil {
		_ = p.cmd.Process.Kill()
	}
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func waitTCPPort(t *testing.T, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("tcp port %d not accepting", port)
}

func rgoshPrintIdentity(t *testing.T, rgoshBin, configDir string) string {
	t.Helper()
	cmd := exec.Command(rgoshBin, "-config", configDir, "-p", "-json")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rgosh -p -json: %v\n%s", err, out)
	}
	var parsed struct {
		Destinations struct {
			Rgosh string `json:"rgosh"`
		} `json:"destinations"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &parsed); err != nil {
		t.Fatalf("parse rgosh identity JSON %q: %v", out, err)
	}
	if len(parsed.Destinations.Rgosh) != 32 {
		t.Fatalf("rgosh destination hash length in %q", out)
	}
	return parsed.Destinations.Rgosh
}

type rgoshRunResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
	elapsed  time.Duration
	timedOut bool
}

func runRgoshClient(ctx context.Context, rgoshBin, configDir, destHex string, mirror bool, input []byte, timeout time.Duration, remote ...string) rgoshRunResult {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"-config", configDir, "-N", "-v", "-w", "10"}
	if mirror {
		args = append(args, "-m")
	}
	args = append(args, destHex)
	args = append(args, remote...)
	cmd := exec.CommandContext(runCtx, rgoshBin, args...)
	cmd.Env = append(os.Environ(), "HOME="+configDir)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	err := cmd.Run()
	res := rgoshRunResult{stdout: stdout.String(), stderr: stderr.String(), err: err, elapsed: time.Since(start), timedOut: runCtx.Err() == context.DeadlineExceeded}
	if err == nil {
		return res
	}
	var exitErr *exec.ExitError
	if ok := errors.As(err, &exitErr); ok {
		res.exitCode = exitErr.ExitCode()
	} else {
		res.exitCode = -1
	}
	return res
}

func startHubTopology(t *testing.T, share bool) (string, string, string, string, string, string, *testProcess) {
	t.Helper()
	if testing.Short() {
		t.Skip("live hub test skipped in short mode")
	}
	liveOrSkip(t)
	daemonBin, rgoshBin, pathBin := buildHubTestBinaries(t)
	port := freeTCPPort(t)
	hubCfg := t.TempDir()
	listenerCfg := t.TempDir()
	clientCfg := t.TempDir()
	writeHubDaemonConfig(t, hubCfg, port, share)
	writeHubClientConfig(t, listenerCfg, port, share)
	writeHubClientConfig(t, clientCfg, port, share)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	hub := startTestProcess(t, ctx, "hub", daemonBin, "-config", hubCfg, "-debug", "5")
	waitTCPPort(t, port, 5*time.Second)
	return daemonBin, rgoshBin, pathBin, listenerCfg, clientCfg, hubCfg, hub
}

func waitBufferContains(t *testing.T, b *lockedBuffer, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(b.String(), want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("buffer never contained %q:\n%s", want, b.String())
}

func startVerboseRgoshListener(t *testing.T, ctx context.Context, rgoshBin, configDir string) *testProcess {
	t.Helper()
	return startTestProcess(t, ctx, "verbose rgosh listener", rgoshBin, "-config", configDir, "-v", "-l", "-n", "/bin/sh")
}

func startRgoshListener(t *testing.T, ctx context.Context, rgoshBin, configDir string) *testProcess {
	t.Helper()
	return startTestProcess(t, ctx, "rgosh listener", rgoshBin, "-config", configDir, "-l", "-n", "/bin/sh")
}

type pathRunResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func runRgopath(ctx context.Context, pathBin, configDir, destHex string, timeout time.Duration) pathRunResult {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, pathBin, "-config", configDir, "-w", "10", destHex)
	cmd.Env = append(os.Environ(), "HOME="+configDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := pathRunResult{stdout: stdout.String(), stderr: stderr.String(), err: err, exitCode: -1}
	if err == nil {
		res.exitCode = 0
		return res
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		res.exitCode = exitErr.ExitCode()
	}
	return res
}

func pathFailureDetails(name string, res pathRunResult, listener, hub *testProcess) string {
	return fmt.Sprintf("%s exit=%d err=%v\nstdout:\n%s\nstderr:\n%s\nlistener stderr:\n%s\nhub stderr:\n%s", name, res.exitCode, res.err, res.stdout, res.stderr, listener.stderr.String(), hub.stderr.String())
}

func clientFailureDetails(name string, res rgoshRunResult, listener, hub *testProcess) string {
	return fmt.Sprintf("%s exit=%d err=%v elapsed=%s timedOut=%t\nstdout:\n%s\nstderr:\n%s\nlistener stderr:\n%s\nhub stderr:\n%s", name, res.exitCode, res.err, res.elapsed, res.timedOut, res.stdout, res.stderr, listener.stderr.String(), hub.stderr.String())
}

func TestLiveRgoshHubExitBehavior(t *testing.T) {
	_, rgoshBin, _, listenerCfg, clientCfg, _, hub := startHubTopology(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	destHex := rgoshPrintIdentity(t, rgoshBin, listenerCfg)
	listener := startRgoshListener(t, ctx, rgoshBin, listenerCfg)
	time.Sleep(1200 * time.Millisecond)

	t.Run("mirror exit seven", func(t *testing.T) {
		res := runRgoshClient(ctx, rgoshBin, clientCfg, destHex, true, nil, 25*time.Second, "/bin/sh", "-c", "exit 7")
		if res.exitCode != 7 || res.elapsed >= 15*time.Second || res.timedOut {
			t.Fatalf("mirror exit behavior incorrect: %s", clientFailureDetails("exit seven", res, listener, hub))
		}
	})
	t.Run("without mirror exits zero", func(t *testing.T) {
		res := runRgoshClient(ctx, rgoshBin, clientCfg, destHex, false, nil, 25*time.Second, "/bin/sh", "-c", "exit 7")
		if res.exitCode != 0 || res.elapsed >= 15*time.Second || res.timedOut {
			t.Fatalf("non-mirror exit behavior incorrect: %s", clientFailureDetails("non-mirror", res, listener, hub))
		}
	})
	t.Run("stdout delivered", func(t *testing.T) {
		res := runRgoshClient(ctx, rgoshBin, clientCfg, destHex, true, nil, 25*time.Second, "/bin/echo", "hi")
		if res.exitCode != 0 || !strings.Contains(res.stdout, "hi\n") || res.elapsed >= 15*time.Second || res.timedOut {
			t.Fatalf("stdout delivery incorrect: %s", clientFailureDetails("stdout", res, listener, hub))
		}
	})
	t.Run("piped stdin closes promptly", func(t *testing.T) {
		res := runRgoshClient(ctx, rgoshBin, clientCfg, destHex, true, []byte("input\n"), 25*time.Second, "/bin/sh", "-c", "cat >/dev/null; echo stdin-done")
		if res.exitCode != 0 || !strings.Contains(res.stdout, "stdin-done") || res.elapsed >= 15*time.Second || res.timedOut {
			t.Fatalf("stdin close behavior incorrect: %s", clientFailureDetails("stdin", res, listener, hub))
		}
	})
}

func TestLiveRgoshHubBridgingAndListenerRestart(t *testing.T) {
	_, rgoshBin, pathBin, listenerCfg, clientCfg, _, hub := startHubTopology(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	destHex := rgoshPrintIdentity(t, rgoshBin, listenerCfg)
	listener := startRgoshListener(t, ctx, rgoshBin, listenerCfg)
	time.Sleep(1200 * time.Millisecond)

	first := runRgoshClient(ctx, rgoshBin, clientCfg, destHex, true, nil, 30*time.Second, "/bin/echo", "through-hub")
	if first.exitCode != 0 || !strings.Contains(first.stdout, "through-hub") || first.timedOut {
		t.Fatalf("initial hub-bridged command failed: %s", clientFailureDetails("initial bridged command", first, listener, hub))
	}
	path := runRgopath(ctx, pathBin, clientCfg, destHex, 30*time.Second)
	if path.exitCode != 0 || !strings.Contains(path.stdout, "Path found: 2 hops") {
		t.Fatalf("path did not resolve through hub with two hops: %s", pathFailureDetails("initial path", path, listener, hub))
	}

	stopTestProcess(listener)
	restarted := startRgoshListener(t, ctx, rgoshBin, listenerCfg)
	start := time.Now()
	second := runRgoshClient(ctx, rgoshBin, clientCfg, destHex, true, nil, 45*time.Second, "/bin/echo", "after-restart")
	elapsed := time.Since(start)
	if second.exitCode != 0 || !strings.Contains(second.stdout, "after-restart") || second.timedOut || elapsed >= 45*time.Second {
		t.Fatalf("post-restart hub-bridged command failed after %s: %s", elapsed, clientFailureDetails("post-restart command", second, restarted, hub))
	}
}

func TestLiveRgoshSharedInstanceConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("live shared-instance test skipped in short mode")
	}
	liveOrSkip(t)
	daemonBin, rgoshBin, _ := buildHubTestBinaries(t)
	port := freeTCPPort(t)
	sharedCfg := t.TempDir()
	clientCfg := t.TempDir()
	writeHubDaemonConfig(t, sharedCfg, port, true)
	writeHubClientConfig(t, clientCfg, port, false)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	hub := startTestProcess(t, ctx, "shared hub", daemonBin, "-config", sharedCfg, "-debug", "5")
	waitTCPPort(t, port, 5*time.Second)
	destHex := rgoshPrintIdentity(t, rgoshBin, sharedCfg)

	first := startVerboseRgoshListener(t, ctx, rgoshBin, sharedCfg)
	waitBufferContains(t, first.stderr, "Connected to existing local shared instance", 8*time.Second)
	second := startVerboseRgoshListener(t, ctx, rgoshBin, sharedCfg)
	waitBufferContains(t, second.stderr, "Connected to existing local shared instance", 8*time.Second)
	waitBufferContains(t, hub.stderr, "Conflicting announces for one destination", 8*time.Second)
	for _, proc := range []*testProcess{first, second} {
		if strings.Contains(proc.stderr.String(), `Setting up interface" name=hub`) {
			t.Fatalf("shared-instance client started the configured hub interface:\n%s", proc.stderr.String())
		}
	}
	time.Sleep(4 * time.Second)

	res := runRgoshClient(ctx, rgoshBin, clientCfg, destHex, true, nil, 30*time.Second, "/bin/echo", "shared-conflict-probe")
	if res.exitCode != 0 || !strings.Contains(res.stdout, "shared-conflict-probe") {
		t.Fatalf("client could not reach duplicated shared-instance listeners: %s", clientFailureDetails("shared conflict", res, second, hub))
	}
}
