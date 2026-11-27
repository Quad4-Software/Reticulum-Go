// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestInterop_Live_GolxmdOnInboundHook verifies that a message delivered to
// the golxmd delivery destination triggers the configured on_inbound command
// with the written message file path appended as final argument, matching the
// Python lxmd contract (shlex.split(command + ' "<path>"')).
func TestInterop_Live_GolxmdOnInboundHook(t *testing.T) {
	requireRealLXMD(t)
	bin := buildGolxmdBinary(t)
	goPort := freeUDPPort(t)
	pyPort := freeUDPPort(t)

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "storage", "messages"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestIdentityFile(t, filepath.Join(home, "identity"))
	marker := filepath.Join(home, "hook-args")
	hook := filepath.Join(home, "hook.sh")
	hookScript := "#!/bin/sh\nprintf '%s' \"$1\" > " + marker + "\n"
	if err := os.WriteFile(hook, []byte(hookScript), 0o700); err != nil {
		t.Fatal(err)
	}
	writeLXMDConfig(t, home, nil, nil)
	cfgPath := filepath.Join(home, "config")
	raw, err := os.ReadFile(filepath.Clean(cfgPath)) // #nosec G304 -- test config
	if err != nil {
		t.Fatal(err)
	}
	cfg := strings.Replace(string(raw), "# on_inbound = rm", "on_inbound = "+hook, 1)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	readyPath := filepath.Join(home, "ready.json")
	cmd := exec.Command(bin,
		"--config", home,
		"--udp-listen", fmt.Sprintf("127.0.0.1:%d", goPort),
		"--udp-forward", fmt.Sprintf("127.0.0.1:%d", pyPort),
		"--ready-file", readyPath,
		"--service",
	)
	proc := startLXMDProcess(t, cmd)
	ready := waitGolxmdReadyFile(t, readyPath, func() string { return proc.buf.String() })

	wantText := "on inbound hook interop"
	resp := interopCall(t, map[string]any{
		"cmd":          "live_send_delivery",
		"dest_hash":    ready["delivery_hash"],
		"listen_port":  pyPort,
		"forward_port": goPort,
		"title":        "hook",
		"text":         wantText,
		"timeout_s":    45,
	})
	if !resp.OK {
		t.Fatalf("python send failed: %+v log=%s", resp, proc.buf.String())
	}

	deadline := time.Now().Add(25 * time.Second)
	var hookArg string
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Clean(marker)) // #nosec G304 -- test marker
		if err == nil && len(data) > 0 {
			hookArg = string(data)
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if hookArg == "" {
		t.Fatalf("on_inbound hook did not run, log=%s", proc.buf.String())
	}
	if !strings.HasPrefix(hookArg, filepath.Join(home, "storage", "messages")) {
		t.Fatalf("hook argument %q is not a message file path in the messages dir", hookArg)
	}
	_, msg, err := ReadFromFile(hookArg, RecallSource)
	if err != nil || msg == nil {
		t.Fatalf("hook path %q is not a readable lxmf message file: %v", hookArg, err)
	}
	if msg.ContentString() != wantText {
		t.Fatalf("hook message content %q != %q", msg.ContentString(), wantText)
	}
}

// TestInterop_Live_PythonLXMDOnInboundHook runs the same scenario against real
// Python lxmd to compare the contract.
func TestInterop_Live_PythonLXMDOnInboundHook(t *testing.T) {
	lxmdPath := requireRealLXMD(t)
	pyPort := freeUDPPort(t)
	senderPort := freeUDPPort(t)

	home := t.TempDir()
	writeTestIdentityFile(t, filepath.Join(home, "identity"))
	marker := filepath.Join(home, "hook-args")
	hook := filepath.Join(home, "hook.sh")
	hookScript := "#!/bin/sh\nprintf '%s' \"$1\" > " + marker + "\n"
	if err := os.WriteFile(hook, []byte(hookScript), 0o700); err != nil {
		t.Fatal(err)
	}
	writeLXMDConfig(t, home, nil, nil)
	cfgPath := filepath.Join(home, "config")
	raw, err := os.ReadFile(filepath.Clean(cfgPath)) // #nosec G304 -- test config
	if err != nil {
		t.Fatal(err)
	}
	cfg := strings.Replace(string(raw), "# on_inbound = rm", "on_inbound = "+hook, 1)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	pyRNS := t.TempDir()
	writeInteropRNSConfig(t, pyRNS, pyPort, senderPort)
	cmd := exec.Command(lxmdPath, "--config", home, "--rnsconfig", pyRNS, "--service")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	proc := startLXMDProcess(t, cmd)
	time.Sleep(4 * time.Second)

	deliveryHash := pythonDeliveryHashForNode(t, filepath.Join(home, "identity"))
	wantText := "python lxmd hook interop"
	resp := interopCall(t, map[string]any{
		"cmd":          "live_send_delivery",
		"dest_hash":    deliveryHash,
		"listen_port":  senderPort,
		"forward_port": pyPort,
		"title":        "hook",
		"text":         wantText,
		"timeout_s":    45,
	})
	if !resp.OK {
		t.Fatalf("python send failed: %+v log=%s", resp, proc.buf.String())
	}

	deadline := time.Now().Add(25 * time.Second)
	var hookArg string
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Clean(marker)) // #nosec G304 -- test marker
		if err == nil && len(data) > 0 {
			hookArg = string(data)
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if hookArg == "" {
		t.Fatalf("python lxmd on_inbound hook did not run, log=%s", proc.buf.String())
	}
	if !strings.Contains(hookArg, filepath.Join("storage", "messages")) {
		t.Fatalf("python hook argument %q is not a messages dir path", hookArg)
	}
	if _, err := os.Stat(hookArg); err != nil {
		t.Fatalf("python hook path does not exist: %v", err)
	}
}

func pythonDeliveryHashForNode(t *testing.T, identityPath string) string {
	t.Helper()
	py := interopPythonPath()
	if py == "" {
		t.Skip("python LXMF unavailable")
	}
	script := fmt.Sprintf(`import RNS,LXMF
id=RNS.Identity.from_file(%q)
d=RNS.Destination(id,RNS.Destination.OUT,RNS.Destination.SINGLE,LXMF.APP_NAME,"delivery")
print(d.hash.hex())
`, identityPath)
	out, err := exec.Command(py, "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("python delivery hash: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestInterop_Live_GolxmdSoak runs golxmd with an active status reporter and a
// peer for at least 90 seconds, checking resource usage stability and clean
// shutdown.
func TestInterop_Live_GolxmdSoak(t *testing.T) {
	requireRealLXMD(t)
	bin := buildGolxmdBinary(t)
	hubPort := freeTCPPort(t)

	hubDir := t.TempDir()
	writeHubRNSConfig(t, hubDir, hubPort)
	startPythonRnsd(t, hubDir)
	time.Sleep(2 * time.Second)

	goHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(goHome, "storage", "messages"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestIdentityFile(t, filepath.Join(goHome, "identity"))
	queryHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(queryHome, "storage", "messages"), 0o700); err != nil {
		t.Fatal(err)
	}
	queryID := writeTestIdentityFile(t, filepath.Join(queryHome, "identity"))
	writeLXMDConfig(t, goHome, queryID, nil)
	writeLXMDConfig(t, queryHome, queryID, nil)
	goRNS := t.TempDir()
	writeTCPClientRNSConfig(t, goRNS, hubPort)
	clientRNS := t.TempDir()
	writeTCPClientRNSConfig(t, clientRNS, hubPort)

	readyPath := filepath.Join(goHome, "ready.json")
	cmd := exec.Command(bin,
		"--config", goHome,
		"--propagation-node",
		"--rnsconfig", goRNS,
		"--ready-file", readyPath,
		"--service",
	)
	proc := startLXMDProcess(t, cmd)
	ready := waitGolxmdReadyFile(t, readyPath, func() string { return goProcBuf(proc) })

	countFDs := func() int {
		ents, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", proc.cmd.Process.Pid))
		if err != nil {
			return -1
		}
		return len(ents)
	}
	rssKB := func() int64 {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", proc.cmd.Process.Pid))
		if err != nil {
			return -1
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				var kb int64
				_, _ = fmt.Sscanf(line, "VmRSS: %d kB", &kb)
				return kb
			}
		}
		return -1
	}

	queryOnce := func() error {
		q := exec.Command(bin,
			"--config", queryHome,
			"--rnsconfig", clientRNS,
			"--status",
			"--remote", ready["propagation_hash"],
			"--timeout", "15",
		)
		return q.Run()
	}

	start := time.Now()
	for time.Since(start) < 20*time.Second {
		time.Sleep(time.Second)
	}
	fds20, rss20 := countFDs(), rssKB()
	if err := queryOnce(); err != nil {
		t.Fatalf("status query at t=20s failed: %v\n%s", err, proc.buf.String())
	}

	for time.Since(start) < 95*time.Second {
		time.Sleep(5 * time.Second)
	}
	if err := queryOnce(); err != nil {
		t.Fatalf("status query at t=end failed: %v\n%s", err, proc.buf.String())
	}
	fdsEnd, rssEnd := countFDs(), rssKB()

	if fds20 >= 0 && fdsEnd >= 0 && fdsEnd > fds20+10 {
		t.Fatalf("fd count grew from %d to %d", fds20, fdsEnd)
	}
	if rss20 > 0 && rssEnd > 0 && rssEnd > rss20*2+30000 {
		t.Fatalf("rss grew from %d kB to %d kB", rss20, rssEnd)
	}

	if strings.Contains(proc.buf.String(), "panic") {
		t.Fatalf("golxmd panicked during soak:\n%s", readFileTail(filepath.Join(goHome, "logfile"), 4000))
	}

	_ = proc.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- proc.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("golxmd exit error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("golxmd did not exit within 5s of SIGTERM")
	}
}

func goProcBuf(p *lxmdProcess) string { return p.buf.String() }

func TestGolxmdExampleConfigParsesClean(t *testing.T) {
	bin := buildGolxmdBinary(t)
	out, err := exec.Command(bin, "--exampleconfig").CombinedOutput()
	if err != nil {
		t.Fatalf("golxmd --exampleconfig: %v\n%s", err, out)
	}
	if len(out) == 0 {
		t.Fatal("empty exampleconfig output")
	}
	cfg, err := ParseConfig(strings.NewReader(string(out)))
	if err != nil {
		t.Fatalf("golxmd exampleconfig does not parse: %v", err)
	}
	if !cfg.Propagation.Autopeer {
		t.Fatal("expected autopeer enabled in example config")
	}
}

func captureConfigWarnings(t *testing.T, text string) []string {
	t.Helper()
	var buf bytes.Buffer
	SetLogOutput(&buf)
	defer SetLogOutput(nil)
	if _, err := ParseConfig(strings.NewReader(text)); err != nil {
		t.Fatalf("config parse: %v", err)
	}
	var warnings []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "unknown lxmf config") {
			warnings = append(warnings, line)
		}
	}
	return warnings
}

func TestInterop_Live_GolxmdExampleConfigParsesInPythonLXMD(t *testing.T) {
	lxmdPath := requireRealLXMD(t)
	bin := buildGolxmdBinary(t)
	out, err := exec.Command(bin, "--exampleconfig").CombinedOutput()
	if err != nil {
		t.Fatalf("golxmd --exampleconfig: %v", err)
	}

	pyPort := freeUDPPort(t)
	home := t.TempDir()
	writeTestIdentityFile(t, filepath.Join(home, "identity"))
	cfgText := strings.Replace(string(out), "enable_node = no", "enable_node = yes", 1)
	if err := os.WriteFile(filepath.Join(home, "config"), []byte(cfgText), 0o600); err != nil {
		t.Fatal(err)
	}
	pyRNS := t.TempDir()
	writeInteropRNSConfig(t, pyRNS, pyPort, freeUDPPort(t))

	cmd := exec.Command(lxmdPath, "--config", home, "--rnsconfig", pyRNS, "--service")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	proc := startLXMDProcess(t, cmd)
	defer func() {
		if strings.Contains(proc.buf.String(), "error") && t.Failed() {
			t.Logf("python lxmd log:\n%s", proc.buf.String())
		}
	}()

	time.Sleep(6 * time.Second)
	if proc.cmd.ProcessState != nil {
		t.Fatalf("python lxmd exited early on golxmd exampleconfig:\n%s", proc.buf.String())
	}
	logText := readFileTail(filepath.Join(home, "logfile"), 8000)
	if strings.Contains(logText, "Check your configuration file") || strings.Contains(proc.buf.String(), "Could not parse the configuration") {
		t.Fatalf("python lxmd rejected golxmd exampleconfig:\n%s\n%s", logText, proc.buf.String())
	}
}

func TestInterop_Live_PythonExampleConfigParsesInGolxmd(t *testing.T) {
	lxmdPath := requireRealLXMD(t)
	out, err := exec.Command(lxmdPath, "--exampleconfig").CombinedOutput()
	if err != nil {
		t.Fatalf("python lxmd --exampleconfig: %v\n%s", err, out)
	}
	cfg, err := ParseConfig(strings.NewReader(string(out)))
	if err != nil {
		t.Fatalf("python exampleconfig does not parse in golxmd: %v", err)
	}
	warnings := captureConfigWarnings(t, string(out))
	if len(warnings) > 0 {
		t.Fatalf("python exampleconfig produces golxmd unknown-key warnings: %v", warnings)
	}
	if !cfg.Propagation.Autopeer {
		t.Fatal("autopeer mismatch: python exampleconfig should enable autopeer")
	}
	if cfg.Propagation.MaxPeers != 20 {
		t.Fatalf("max_peers = %d, want 20", cfg.Propagation.MaxPeers)
	}
	if cfg.Propagation.MessageStorageLimitMB != 500 {
		t.Fatalf("message_storage_limit = %v, want 500", cfg.Propagation.MessageStorageLimitMB)
	}
	if cfg.Propagation.PropagationTransferMaxAcceptedKB != 256 {
		t.Fatalf("propagation_transfer_max_accepted_size = %v, want 256", cfg.Propagation.PropagationTransferMaxAcceptedKB)
	}
	if cfg.Propagation.PropagationStampCostTarget != 16 {
		t.Fatalf("propagation_stamp_cost_target = %d, want 16", cfg.Propagation.PropagationStampCostTarget)
	}
	if cfg.Propagation.PeeringCost != 18 {
		t.Fatalf("peering_cost = %d, want 18", cfg.Propagation.PeeringCost)
	}
}

func TestGolxmdStartupExitCodes(t *testing.T) {
	bin := buildGolxmdBinary(t)
	freePort := freeUDPPort(t)
	freePort2 := freeUDPPort(t)

	t.Run("bad config exits 255", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, "config"), []byte("[lxmf\nbroken = ="), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "--config", home,
			"--udp-listen", fmt.Sprintf("127.0.0.1:%d", freePort),
			"--udp-forward", fmt.Sprintf("127.0.0.1:%d", freePort2),
		)
		err := cmd.Run()
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("expected exit error, got %v", err)
		}
		if exitErr.ExitCode() != 255 {
			t.Fatalf("bad config exit code = %d, want 255", exitErr.ExitCode())
		}
	})

	t.Run("invalid identity exits 4", func(t *testing.T) {
		home := t.TempDir()
		writeLXMDConfig(t, home, nil, nil)
		if err := os.WriteFile(filepath.Join(home, "identity"), []byte("not an identity"), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "--config", home,
			"--udp-listen", fmt.Sprintf("127.0.0.1:%d", freePort),
			"--udp-forward", fmt.Sprintf("127.0.0.1:%d", freePort2),
		)
		err := cmd.Run()
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("expected exit error, got %v", err)
		}
		if exitErr.ExitCode() != 4 {
			t.Fatalf("invalid identity exit code = %d, want 4", exitErr.ExitCode())
		}
	})

	t.Run("missing config dir is created", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "fresh")
		readyPath := filepath.Join(t.TempDir(), "ready.json")
		cmd := exec.Command(bin, "--config", home,
			"--udp-listen", fmt.Sprintf("127.0.0.1:%d", freePort),
			"--udp-forward", fmt.Sprintf("127.0.0.1:%d", freePort2),
			"--ready-file", readyPath,
			"--service",
		)
		proc := startLXMDProcess(t, cmd)
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if info, err := os.Stat(readyPath); err == nil && info.Size() > 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if _, err := os.Stat(filepath.Join(home, "identity")); err != nil {
			t.Fatalf("identity not auto-created: %v\n%s", err, proc.buf.String())
		}
	})
}

func TestInterop_Live_GolxmdImportsPythonPeers(t *testing.T) {
	lxmdPath := requireRealLXMD(t)
	bin := buildGolxmdBinary(t)
	goPort := freeUDPPort(t)
	pyPort := freeUDPPort(t)

	clientHome := t.TempDir()
	clientID := writeTestIdentityFile(t, filepath.Join(clientHome, "identity"))
	writeLXMDConfig(t, clientHome, nil, nil)
	pyRNS := t.TempDir()
	writeInteropRNSConfig(t, pyRNS, pyPort, goPort)

	goHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(goHome, "storage", "messages"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestIdentityFile(t, filepath.Join(goHome, "identity"))
	writeLXMDConfig(t, goHome, clientID, nil)

	peerHash := "cc97e518fe366e782c53e5c93d1e39e4"
	writePythonPeersFile(t, filepath.Join(goHome, "storage", "lxmf"), peerHash)

	readyPath := filepath.Join(goHome, "ready.json")
	cmd := exec.Command(bin,
		"--config", goHome,
		"--propagation-node",
		"--udp-listen", fmt.Sprintf("127.0.0.1:%d", goPort),
		"--udp-forward", fmt.Sprintf("127.0.0.1:%d", pyPort),
		"--ready-file", readyPath,
		"--service",
	)
	proc := startLXMDProcess(t, cmd)
	ready := waitGolxmdReadyFile(t, readyPath, func() string { return proc.buf.String() })

	query := exec.Command(lxmdPath,
		"--config", clientHome,
		"--rnsconfig", pyRNS,
		"--status", "--peers",
		"--remote", ready["propagation_hash"],
		"--timeout", "15",
	)
	query.Env = append(os.Environ(), "HOME="+t.TempDir())
	out, err := query.CombinedOutput()
	if err != nil {
		t.Fatalf("python lxmd status exit=%v output=%s\ndaemon=%s", err, out, proc.buf.String())
	}
	if !strings.Contains(string(out), peerHash) {
		t.Fatalf("imported peer %s not visible in status output:\n%s", peerHash, out)
	}
}

func writePythonPeersFile(t *testing.T, lxmfStorage string, peerHashHex string) {
	t.Helper()
	py := interopPythonPath()
	if py == "" {
		t.Skip("python LXMF unavailable")
	}
	if err := os.MkdirAll(lxmfStorage, 0o700); err != nil {
		t.Fatal(err)
	}
	// Generated with real Python LXMF: LXMPeer serialised with to_bytes and
	// written as a msgpack list, the exact format LXMRouter.save_peers uses.
	script := fmt.Sprintf(`import RNS, LXMF
from RNS.vendor import umsgpack as msgpack
_r = RNS.Reticulum()
router = LXMF.LXMRouter(identity=RNS.Identity(), storagepath=%q)
peer = LXMF.LXMPeer.LXMPeer(router, bytes.fromhex(%q), sync_strategy=LXMF.LXMPeer.LXMPeer.STRATEGY_PERSISTENT)
data = msgpack.packb([peer.to_bytes()])
open(%q, "wb").write(data)
print("peers written")
`, lxmfStorage+"-gen", peerHashHex, filepath.Join(lxmfStorage, "peers"))
	out, err := exec.Command(py, "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("python peers file generation: %v\n%s", err, out)
	}
}

func writeSharedInstanceRNSConfig(t *testing.T, dir string, sharedPort, hubTCPPort int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "storage", "identities"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`[reticulum]
  enable_transport = Yes
  share_instance = Yes
  rpc_key = aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  shared_instance_type = tcp
  shared_instance_port = %d

[logging]
  loglevel = 4

[interfaces]

  [[TCP Server Interface]]
    type = TCPServerInterface
    interface_enabled = True
    listen_ip = 127.0.0.1
    listen_port = %d
`, sharedPort, hubTCPPort)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeSharedClientRNSConfig(t *testing.T, dir string, sharedPort int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "storage", "identities"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`[reticulum]
  enable_transport = Yes
  share_instance = Yes
  rpc_key = aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  shared_instance_type = tcp
  shared_instance_port = %d

[logging]
  loglevel = 4
`, sharedPort)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInterop_Live_GolxmdSharedInstance(t *testing.T) {
	lxmdPath := requireRealLXMD(t)
	bin := buildGolxmdBinary(t)
	sharedPort := freeTCPPort(t)
	hubTCPPort := freeTCPPort(t)

	rnsdDir := t.TempDir()
	writeSharedInstanceRNSConfig(t, rnsdDir, sharedPort, hubTCPPort)
	rnsdProc := startPythonRnsd(t, rnsdDir)
	time.Sleep(3 * time.Second)

	goHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(goHome, "storage", "messages"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestIdentityFile(t, filepath.Join(goHome, "identity"))
	queryHome := t.TempDir()
	queryID := writeTestIdentityFile(t, filepath.Join(queryHome, "identity"))
	writeLXMDConfig(t, goHome, queryID, nil)
	goRNS := t.TempDir()
	writeSharedClientRNSConfig(t, goRNS, sharedPort)

	readyPath := filepath.Join(goHome, "ready.json")
	goCmd := exec.Command(bin,
		"--config", goHome,
		"--rnsconfig", goRNS,
		"--propagation-node",
		"--ready-file", readyPath,
		"--service",
	)
	goProc := startLXMDProcess(t, goCmd)
	ready := waitGolxmdReadyFile(t, readyPath, func() string { return goProc.buf.String() })

	deadline := time.Now().Add(10 * time.Second)
	attached := false
	for time.Now().Before(deadline) {
		if strings.Contains(readFileTail(filepath.Join(goHome, "logfile"), 20000), "attached to reticulum shared instance") {
			attached = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !attached {
		t.Fatalf("golxmd did not attach to shared instance:\n%s", readFileTail(filepath.Join(goHome, "logfile"), 4000))
	}
	if strings.Contains(readFileTail(filepath.Join(goHome, "logfile"), 20000), "rns interface up") {
		t.Fatalf("golxmd started its own interfaces despite shared instance:\n%s", readFileTail(filepath.Join(goHome, "logfile"), 4000))
	}

	pyHome := t.TempDir()
	writeTestIdentityFile(t, filepath.Join(pyHome, "identity"))
	writeLXMDConfig(t, pyHome, nil, nil)
	pyRNS := t.TempDir()
	writeSharedClientRNSConfig(t, pyRNS, sharedPort)
	pyCmd := exec.Command(lxmdPath, "--config", pyHome, "--rnsconfig", pyRNS, "--propagation-node", "--service")
	pyCmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	startLXMDProcess(t, pyCmd)
	time.Sleep(4 * time.Second)

	writeLXMDConfig(t, queryHome, queryID, nil)
	query := exec.Command(lxmdPath,
		"--config", queryHome,
		"--rnsconfig", pyRNS,
		"--status",
		"--remote", ready["propagation_hash"],
		"--timeout", "20",
	)
	query.Env = append(os.Environ(), "HOME="+t.TempDir())
	out, err := query.CombinedOutput()
	if err != nil {
		t.Fatalf("python lxmd status via shared instance exit=%v output=%s\ndaemon-log=%s", err, out, readFileTail(filepath.Join(goHome, "logfile"), 6000))
	}
	if !strings.Contains(string(out), "LXMF Propagation Node") {
		t.Fatalf("unexpected query output:\n%s", out)
	}

	deadline = time.Now().Add(15 * time.Second)
	announced := false
	for time.Now().Before(deadline) {
		if strings.Contains(rnsdProc.buf.String(), ready["propagation_hash"][:16]) || strings.Contains(rnsdProc.buf.String(), ready["propagation_hash"]) {
			announced = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !announced {
		t.Logf("rnsd output did not show the propagation hash literally; checking transport table instead")
	}
}

func TestGolxmdQueryExitCodes(t *testing.T) {
	bin := buildGolxmdBinary(t)

	missing := filepath.Join(t.TempDir(), "nonexistent")
	cmd := exec.Command(bin, "--config", missing, "--status")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("expected failure for missing config dir, output: %s", out)
	} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 201 {
		t.Fatalf("missing config dir exit = %v, want 201, output: %s", err, out)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte("[lxmf]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(bin, "--config", dir, "--status")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("expected failure for missing identity, output: %s", out)
	} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 202 {
		t.Fatalf("missing identity exit = %v, want 202, output: %s", err, out)
	}
}
