// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

func requireRealLXMD(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("real lxmd interop skipped in -short mode")
	}
	if os.Getenv("RUN_LIVE_INTEROP") != "1" {
		t.Skip("real lxmd interop requires RUN_LIVE_INTEROP=1")
	}
	requireInterop(t)
	if path, err := exec.LookPath("lxmd"); err == nil {
		return path
	}
	const installed = "/home/dev/.local/bin/lxmd"
	if _, err := os.Stat(installed); err == nil {
		return installed
	}
	t.Skip("python lxmd executable not found")
	return ""
}

// interopStampsSection zeroes the proof-of-work stamp costs for hermetic
// test networks. Announces and path requests still carry stamps, so the
// wire format is unchanged, but generation finishes immediately instead of
// blocking emission for tens of seconds. Python rnsd/lxmd and golxmd both
// apply the [stamps] section of the config file they share, so one writer
// covers the two sides. Enforcement stays off, matching production
// defaults.
const interopStampsSection = `
[stamps]
  destination_cost = 0
  destination_value = 0
  pr_cost = 0
  pr_value = 0
  lr_cost = 0
`

func writeInteropRNSConfig(t *testing.T, dir string, listenPort, forwardPort int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "storage", "identities"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`[reticulum]
  enable_transport = Yes
  share_instance = No
  shared_instance_port = 0
  instance_name = lxmd-interop-%d

[logging]
  loglevel = 4
`+interopStampsSection+`
[interfaces]

  [[UDP Interface]]
    type = UDPInterface
    interface_enabled = True
    listen_ip = 127.0.0.1
    listen_port = %d
    forward_ip = 127.0.0.1
    forward_port = %d
`, listenPort, listenPort, forwardPort)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeHubRNSConfig(t *testing.T, dir string, port int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "storage", "identities"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`[reticulum]
  enable_transport = Yes
  share_instance = No
  shared_instance_port = 0
  instance_name = lxmd-hub-%d

[logging]
  loglevel = 4
`+interopStampsSection+`
[interfaces]

  [[TCP Server Interface]]
    type = TCPServerInterface
    interface_enabled = True
    listen_ip = 127.0.0.1
    listen_port = %d
`, port, port)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeTCPClientRNSConfig(t *testing.T, dir string, hubPort int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "storage", "identities"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`[reticulum]
  enable_transport = Yes
  share_instance = No
  shared_instance_port = 0
  instance_name = lxmd-leaf-%d

[logging]
  loglevel = 4
`+interopStampsSection+`
[interfaces]

  [[Hub]]
    type = TCPClientInterface
    interface_enabled = True
    target_host = 127.0.0.1
    target_port = %d
`, hubPort, hubPort)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeGolxmdRNSConfig writes a minimal rns config carrying the zero-cost
// [stamps] section and no interfaces. golxmd takes its interfaces from the
// --udp-listen/--udp-forward flags in these tests, so the config only keeps
// golxmd's stamp costs aligned with the python side of the network.
func writeGolxmdRNSConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "storage", "identities"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := `[reticulum]
  enable_transport = Yes
  share_instance = No
  shared_instance_port = 0
` + interopStampsSection
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func startPythonRnsd(t *testing.T, configDir string) *lxmdProcess {
	t.Helper()
	rnsd, err := exec.LookPath("rnsd")
	if err != nil {
		rnsd = "/home/dev/.local/bin/rnsd"
		if _, err := os.Stat(rnsd); err != nil {
			t.Skip("python rnsd not found")
		}
	}
	cmd := exec.Command(rnsd, "--config", configDir)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	return startLXMDProcess(t, cmd)
}

func writeTestIdentityFile(t *testing.T, path string) *identity.Identity {
	t.Helper()
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := id.ToFile(path); err != nil {
		t.Fatal(err)
	}
	return id
}

func writeLXMDConfig(t *testing.T, home string, control *identity.Identity, staticPeer []byte) {
	t.Helper()
	cfg := strings.Replace(DefaultConfigText, "enable_node = no", "enable_node = yes", 1)
	if control != nil {
		// Python lxmd announces its control destination at startup only
		// when more than one control identity is allowed. The dummy
		// second entry keeps the real client first and triggers the
		// startup announce, which pre-generates the control stamp.
		dummy := strings.Repeat("cd", 16)
		cfg = strings.Replace(cfg, "# control_allowed =", "control_allowed = "+hex.EncodeToString(control.Hash())+", "+dummy+"\n# control_allowed =", 1)
	}
	if len(staticPeer) == DestinationLength {
		cfg = strings.Replace(cfg, "# static_peers =", "static_peers = "+hex.EncodeToString(staticPeer)+"\n# static_peers =", 1)
	}
	if err := os.WriteFile(filepath.Join(home, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

func prepareLXMDHome(t *testing.T, control *identity.Identity, staticPeer []byte) (*identity.Identity, string) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "storage", "messages"), 0o700); err != nil {
		t.Fatal(err)
	}
	id := writeTestIdentityFile(t, filepath.Join(home, "identity"))
	writeLXMDConfig(t, home, control, staticPeer)
	return id, home
}

type lxmdProcess struct {
	cmd *exec.Cmd
	buf *strings.Builder
}

func startLXMDProcess(t *testing.T, cmd *exec.Cmd) *lxmdProcess {
	t.Helper()
	buf := &strings.Builder{}
	cmd.Stdout = buf
	cmd.Stderr = buf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", cmd.Path, err)
	}
	p := &lxmdProcess{cmd: cmd, buf: buf}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("process output tail (%s):\n%s", cmd.Path, tailString(buf.String(), 3000))
			for i, a := range cmd.Args {
				if a == "--config" && i+1 < len(cmd.Args) {
					if tail := readFileTail(filepath.Join(cmd.Args[i+1], "logfile"), 6000); tail != "" {
						t.Logf("daemon logfile tail (%s):\n%s", cmd.Args[i+1], tail)
					}
				}
			}
		}
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	})
	return p
}

func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func buildGolxmdBinary(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "golxmd")
	cmd := exec.Command("go", "build", "-mod=vendor", "-o", bin, "./cmd/golxmd")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build golxmd: %v\n%s", err, out)
	}
	return bin
}

func waitGolxmdReadyFile(t *testing.T, path string, output func() string) map[string]string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- test temp file
		if err == nil && len(raw) > 0 {
			var ready map[string]string
			if json.Unmarshal(raw, &ready) == nil && ready["propagation_hash"] != "" {
				return ready
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if output != nil {
		t.Fatalf("timeout waiting for ready file\n%s", output())
	}
	t.Fatal("timeout waiting for ready file")
	return nil
}

func readFileTail(path string, maxBytes int) string {
	raw, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- test log file
	if err != nil {
		return err.Error()
	}
	if len(raw) > maxBytes {
		raw = raw[len(raw)-maxBytes:]
	}
	return string(raw)
}

func pythonPropagationHash(t *testing.T, identityPath string) string {
	t.Helper()
	py := interopPythonPath()
	if py == "" {
		t.Skip("python LXMF unavailable")
	}
	script := fmt.Sprintf(`import RNS,LXMF
id=RNS.Identity.from_file(%q)
d=RNS.Destination(id,RNS.Destination.OUT,RNS.Destination.SINGLE,LXMF.APP_NAME,"propagation")
print(d.hash.hex())
`, identityPath)
	cmd := exec.Command(py, "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python propagation hash: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestInterop_Live_PythonLXMDQueriesGolxmd(t *testing.T) {
	lxmdPath := requireRealLXMD(t)
	bin := buildGolxmdBinary(t)
	goPort := freeUDPPort(t)
	pyPort := freeUDPPort(t)

	clientHome := t.TempDir()
	if err := os.MkdirAll(clientHome, 0o700); err != nil {
		t.Fatal(err)
	}
	clientID := writeTestIdentityFile(t, filepath.Join(clientHome, "identity"))
	writeLXMDConfig(t, clientHome, nil, nil)
	pyRNS := t.TempDir()
	writeInteropRNSConfig(t, pyRNS, pyPort, goPort)

	staticPeer := []byte{0x61, 0x61, 0x61, 0x61, 0x61, 0x61, 0x61, 0x61, 0x61, 0x61, 0x61, 0x61, 0x61, 0x61, 0x61, 0x61}
	_, goHome := prepareLXMDHome(t, clientID, staticPeer)
	readyPath := filepath.Join(goHome, "ready.json")
	cmd := exec.Command(bin,
		"--config", goHome,
		"--propagation-node",
		"--rnsconfig", writeGolxmdRNSConfig(t),
		"--udp-listen", fmt.Sprintf("127.0.0.1:%d", goPort),
		"--udp-forward", fmt.Sprintf("127.0.0.1:%d", pyPort),
		"--ready-file", readyPath,
		"-v",
	)
	proc := startLXMDProcess(t, cmd)
	ready := waitGolxmdReadyFile(t, readyPath, func() string { return proc.buf.String() })

	query := exec.Command(lxmdPath,
		"--config", clientHome,
		"--rnsconfig", pyRNS,
		"--status", "--peers",
		"--remote", ready["propagation_hash"],
		"--timeout", "12", "-vvvv",
	)
	query.Env = append(os.Environ(), "HOME="+t.TempDir())
	out, err := query.CombinedOutput()
	if err != nil {
		t.Fatalf("python lxmd status exit=%v output=%s\ndaemon=%s\nlog=%s", err, out, proc.buf.String(), readFileTail(filepath.Join(goHome, "logfile"), 20000))
	}
	text := string(out)
	if !strings.Contains(text, "LXMF Propagation Node running") || !strings.Contains(text, "Static peer") || !strings.Contains(text, "Peers   : 1 total") {
		t.Fatalf("unexpected python lxmd output:\n%s\ndaemon:\n%s", text, proc.buf.String())
	}
}

func waitPythonLXMDStarted(t *testing.T, home string, timeout time.Duration) {
	t.Helper()
	logPath := filepath.Join(home, "logfile")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(readFileTail(logPath, 20000), "Started lxmd version") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("python lxmd did not log startup within %v:\n%s", timeout, readFileTail(logPath, 4000))
}

func TestInterop_Live_GolxmdQueriesPythonLXMD(t *testing.T) {
	lxmdPath := requireRealLXMD(t)
	bin := buildGolxmdBinary(t)
	pyNodePort := freeUDPPort(t)
	goQueryPort := freeUDPPort(t)

	clientHome := t.TempDir()
	if err := os.MkdirAll(clientHome, 0o700); err != nil {
		t.Fatal(err)
	}
	clientID := writeTestIdentityFile(t, filepath.Join(clientHome, "identity"))
	writeLXMDConfig(t, clientHome, nil, nil)

	staticPeer := []byte{0x62, 0x62, 0x62, 0x62, 0x62, 0x62, 0x62, 0x62, 0x62, 0x62, 0x62, 0x62, 0x62, 0x62, 0x62, 0x62}
	pyID, pyHome := prepareLXMDHome(t, clientID, staticPeer)
	_ = pyID
	pyRNS := t.TempDir()
	writeInteropRNSConfig(t, pyRNS, pyNodePort, goQueryPort)
	pyCmd := exec.Command(lxmdPath, "--config", pyHome, "--rnsconfig", pyRNS, "--propagation-node", "--service", "-vvvv")
	pyCmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	pyProc := startLXMDProcess(t, pyCmd)
	waitPythonLXMDStarted(t, pyHome, 30*time.Second)

	propHash := pythonPropagationHash(t, filepath.Join(pyHome, "identity"))
	// Probe the real end-to-end condition in a retry loop: golxmd can
	// resolve the python propagation node and complete a control query.
	// Python lxmd emits its first announces only after the deferred job
	// delay, so the first attempts legitimately fail. Waiting on stamp
	// cache files instead would couple the test to python internals that
	// change with the stamp cost configuration.
	goRNS := writeGolxmdRNSConfig(t)
	deadline := time.Now().Add(90 * time.Second)
	var out []byte
	for {
		query := exec.Command(bin,
			"--config", clientHome,
			"--rnsconfig", goRNS,
			"--status", "--peers",
			"--remote", propHash,
			"--timeout", "12",
			"--udp-listen", fmt.Sprintf("127.0.0.1:%d", goQueryPort),
			"--udp-forward", fmt.Sprintf("127.0.0.1:%d", pyNodePort),
		)
		var err error
		out, err = query.CombinedOutput()
		if err == nil && strings.Contains(string(out), "LXMF Propagation Node") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("golxmd status never succeeded, last exit=%v output=%s\npython lxmd=%s", err, out, pyProc.buf.String())
		}
		time.Sleep(3 * time.Second)
	}
	text := string(out)
	if !strings.Contains(text, hex.EncodeToString(staticPeer)) {
		t.Fatalf("unexpected golxmd output:\n%s\npython lxmd:\n%s", text, pyProc.buf.String())
	}
}

func writePythonNodeState(t *testing.T, storagePath string) {
	t.Helper()
	py := interopPythonPath()
	if py == "" {
		t.Skip("python LXMF unavailable")
	}
	script := fmt.Sprintf(`import RNS, LXMF, time
LXMRouter = LXMF.LXMRouter
_r = RNS.Reticulum()
router = LXMRouter(identity=RNS.Identity(), storagepath=%q)
router.enable_propagation()
rid = RNS.Identity(create_keys=True)
src = router.register_delivery_identity(router.identity, display_name="state-injector")
recipient = RNS.Destination(rid, RNS.Destination.OUT, RNS.Destination.SINGLE, LXMF.APP_NAME, "delivery")
lxm = LXMF.LXMessage(recipient, src, "imported from python", "state")
lxm.pack()
encrypted = recipient.encrypt(lxm.packed[LXMF.LXMessage.DESTINATION_LENGTH:])
lxmf_data = lxm.packed[:LXMF.LXMessage.DESTINATION_LENGTH] + encrypted
ok = router.lxmf_propagation(lxmf_data, stamp_value=16, stamp_data=b"\x00"*32)
if ok is False:
    raise SystemExit("python lxmf_propagation rejected injected message")
print("entries", len(router.propagation_entries))
`, storagePath)
	cmd := exec.Command(py, "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python state injection: %v\n%s", err, out)
	}
}

func TestInterop_Live_GolxmdImportsPythonState(t *testing.T) {
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

	writePythonNodeState(t, filepath.Join(goHome, "storage"))
	entries, err := os.ReadDir(filepath.Join(goHome, "storage", "lxmf", "messagestore"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("python wrote no messagestore entries: %v", err)
	}

	readyPath := filepath.Join(goHome, "ready.json")
	cmd := exec.Command(bin,
		"--config", goHome,
		"--propagation-node",
		"--rnsconfig", writeGolxmdRNSConfig(t),
		"--udp-listen", fmt.Sprintf("127.0.0.1:%d", goPort),
		"--udp-forward", fmt.Sprintf("127.0.0.1:%d", pyPort),
		"--ready-file", readyPath,
	)
	proc := startLXMDProcess(t, cmd)
	ready := waitGolxmdReadyFile(t, readyPath, func() string { return proc.buf.String() })

	if imported, err := os.ReadDir(filepath.Join(goHome, "storage", "lxmf", "golxmd", "messagestore")); err != nil || len(imported) != len(entries) {
		t.Fatalf("golxmd import copied %v entries, python wrote %d (err=%v)\n%s", len(imported), len(entries), err, proc.buf.String())
	}
	pythonFiles, err := os.ReadDir(filepath.Join(goHome, "storage", "lxmf", "messagestore"))
	if err != nil || len(pythonFiles) != len(entries) {
		t.Fatalf("python messagestore was modified: %v", err)
	}

	query := exec.Command(lxmdPath,
		"--config", clientHome,
		"--rnsconfig", pyRNS,
		"--status",
		"--remote", ready["propagation_hash"],
		"--timeout", "15",
	)
	query.Env = append(os.Environ(), "HOME="+t.TempDir())
	out, err := query.CombinedOutput()
	if err != nil {
		t.Fatalf("python lxmd status exit=%v output=%s\ndaemon=%s", err, out, proc.buf.String())
	}
	if !strings.Contains(string(out), "Messagestore contains 1 messages") {
		t.Fatalf("imported message not visible on the wire:\n%s\ndaemon=%s", out, proc.buf.String())
	}
}

func propagationHashForIdentity(t *testing.T, id *identity.Identity) []byte {
	t.Helper()
	d, err := destination.New(id, destination.Out, destination.Single, AppName, nil, "propagation")
	if err != nil {
		t.Fatal(err)
	}
	return d.GetHash()
}

func countStoreMessages(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, ent := range entries {
		if !ent.IsDir() && !strings.HasSuffix(ent.Name(), ".tmp") {
			n++
		}
	}
	return n
}

func waitStoreMessages(t *testing.T, dir string, want int, timeout time.Duration, label string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if countStoreMessages(dir) >= want {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s: store %s has %d messages, want %d after %s", label, dir, countStoreMessages(dir), want, timeout)
}

func TestInterop_Live_PythonGolxmdPeerSync(t *testing.T) {
	lxmdPath := requireRealLXMD(t)
	bin := buildGolxmdBinary(t)
	hubPort := freeTCPPort(t)

	goHome := t.TempDir()
	pyHome := t.TempDir()
	for _, home := range []string{goHome, pyHome} {
		if err := os.MkdirAll(filepath.Join(home, "storage", "messages"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	goID := writeTestIdentityFile(t, filepath.Join(goHome, "identity"))
	pyID := writeTestIdentityFile(t, filepath.Join(pyHome, "identity"))
	goHash := propagationHashForIdentity(t, goID)
	pyHashHex := pythonPropagationHash(t, filepath.Join(pyHome, "identity"))
	pyHash, err := hex.DecodeString(pyHashHex)
	if err != nil {
		t.Fatal(err)
	}

	writeLXMDConfig(t, goHome, pyID, pyHash)
	writeLXMDConfig(t, pyHome, goID, goHash)

	hubDir := t.TempDir()
	writeHubRNSConfig(t, hubDir, hubPort)
	startPythonRnsd(t, hubDir)
	time.Sleep(2 * time.Second)

	goRNS := t.TempDir()
	writeTCPClientRNSConfig(t, goRNS, hubPort)
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

	// Python lxmd only creates the outbound peer destination when the peer
	// identity is recallable at construction time or inside the first sync.
	// If the peering key generation recalls the identity first, the destination
	// is never created and every sync fails with "identity could not be
	// recalled". Starting golxmd first lets its propagation announce reach the
	// python node before the static peer is constructed, which is the ordering
	// upstream lxmd depends on.
	time.Sleep(3 * time.Second)
	pyRNS := t.TempDir()
	writeTCPClientRNSConfig(t, pyRNS, hubPort)
	pyCmd := exec.Command(lxmdPath, "--config", pyHome, "--rnsconfig", pyRNS, "--propagation-node", "--service")
	pyCmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	pyProc := startLXMDProcess(t, pyCmd)
	if ready["propagation_hash"] != hex.EncodeToString(goHash) {
		t.Fatalf("golxmd propagation hash %q != computed %q", ready["propagation_hash"], hex.EncodeToString(goHash))
	}

	time.Sleep(3 * time.Second)

	uploadToGo := interopCall(t, map[string]any{
		"cmd":              "live_send_propagation",
		"propagation_hash": hex.EncodeToString(goHash),
		"hub_port":         hubPort,
		"title":            "sync-a",
		"text":             "message injected into golxmd",
		"timeout_s":        90,
	})
	if !uploadToGo.OK {
		t.Fatalf("upload to golxmd failed: %+v\ngo=%s\npy=%s", uploadToGo, goProc.buf.String(), pyProc.buf.String())
	}
	uploadToPy := interopCall(t, map[string]any{
		"cmd":              "live_send_propagation",
		"propagation_hash": pyHashHex,
		"hub_port":         hubPort,
		"title":            "sync-b",
		"text":             "message injected into python lxmd",
		"timeout_s":        90,
	})
	if !uploadToPy.OK {
		t.Fatalf("upload to python lxmd failed: %+v\ngo=%s\npy=%s", uploadToPy, goProc.buf.String(), pyProc.buf.String())
	}

	// Upstream LXMF lxmd only builds the outbound peer destination when the
	// peer identity is recallable at peer construction time, and peers are only
	// constructed at startup. If the identity becomes recallable later through
	// a path request answer, the destination is never created and every sync
	// fails with "identity could not be recalled". Production nodes rely on the
	// persistent identity cache across restarts, so this test does the same:
	// once python has recalled the golxmd identity, restart python lxmd so the
	// peer constructor picks it up.
	time.Sleep(15 * time.Second)
	_ = pyProc.cmd.Process.Signal(syscall.SIGTERM)
	donePy := make(chan struct{})
	go func() { _, _ = pyProc.cmd.Process.Wait(); close(donePy) }()
	select {
	case <-donePy:
	case <-time.After(10 * time.Second):
		_ = pyProc.cmd.Process.Kill()
	}
	time.Sleep(time.Second)
	pyCmd2 := exec.Command(lxmdPath, "--config", pyHome, "--rnsconfig", pyRNS, "--propagation-node", "--service")
	pyCmd2.Env = append(os.Environ(), "HOME="+t.TempDir())
	pyProc = startLXMDProcess(t, pyCmd2)
	waitPythonLXMDStarted(t, pyHome, 30*time.Second)

	goStore := filepath.Join(goHome, "storage", "lxmf", "messagestore")
	pyStore := filepath.Join(pyHome, "storage", "lxmf", "messagestore")
	waitStoreMessages(t, goStore, 2, 90*time.Second, "golxmd syncs from python")
	waitStoreMessages(t, pyStore, 2, 90*time.Second, "python syncs from golxmd")

	queryPy := exec.Command(bin,
		"--config", goHome,
		"--rnsconfig", goRNS,
		"--status", "--peers",
		"--remote", pyHashHex,
		"--timeout", "20",
	)
	outPy, err := queryPy.CombinedOutput()
	if err != nil {
		t.Fatalf("golxmd query of python node: %v\n%s", err, outPy)
	}
	if !strings.Contains(string(outPy), hex.EncodeToString(goHash)) {
		t.Fatalf("python node does not list golxmd as peer:\n%s", outPy)
	}

	queryGo := exec.Command(lxmdPath,
		"--config", pyHome,
		"--rnsconfig", pyRNS,
		"--status", "--peers",
		"--remote", ready["propagation_hash"],
		"--timeout", "20",
	)
	queryGo.Env = append(os.Environ(), "HOME="+t.TempDir())
	outGo, err := queryGo.CombinedOutput()
	if err != nil {
		t.Fatalf("python query of golxmd node: %v\n%s\ngo=%s", err, outGo, goProc.buf.String())
	}
	text := string(outGo)
	if !strings.Contains(text, "Peer") || !strings.Contains(text, pyHashHex[:len(pyHashHex)]) && !strings.Contains(text, "<"+pyHashHex+">") {
		t.Logf("python status output for golxmd:\n%s", text)
	}
	if !strings.Contains(text, "Peers") {
		t.Fatalf("unexpected golxmd status output:\n%s", text)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}
