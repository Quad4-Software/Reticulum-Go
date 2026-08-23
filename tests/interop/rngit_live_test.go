// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live rngit interop: Go and Python git-remote-rns over paired UDP.
// Set RUN_LIVE_INTEROP=1 to enable.

package interop

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsgit"
	"quad4/reticulum-go/pkg/rnsutil"
)

func writeRNGitServerConfig(t *testing.T, rngitDir, repoRoot string) {
	t.Helper()
	if err := os.MkdirAll(rngitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rngitDir, "config"), []byte(strings.Join([]string{
		"[repositories]",
		"public = " + repoRoot,
		"[access]",
		"public = rw:all",
		"[rngit]",
		"announce_interval = 0",
		"mirror_interval = 0",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
}

func initBareRepoWithCommit(t *testing.T, barePath string) {
	t.Helper()
	if out, err := rngitGitCmd("git", "init", "--bare", barePath).CombinedOutput(); err != nil {
		t.Fatalf("bare init: %v %s", err, out)
	}
	work := t.TempDir()
	gitEnv := append(rnsgit.GitCleanEnv(),
		"GIT_AUTHOR_NAME=interop",
		"GIT_AUTHOR_EMAIL=interop@test",
		"GIT_COMMITTER_NAME=interop",
		"GIT_COMMITTER_EMAIL=interop@test",
	)
	if out, err := rngitGitCmd("git", "clone", barePath, work).CombinedOutput(); err != nil {
		t.Fatalf("clone work: %v %s", err, out)
	}
	readme := filepath.Join(work, "README")
	if err := os.WriteFile(readme, []byte("rngit interop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "README"},
		{"git", "commit", "-m", "init"},
		{"git", "push", "origin", "HEAD"},
	} {
		cmd := rngitGitCmd(args...)
		cmd.Dir = work
		cmd.Env = gitEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
}

func startGoRngitNode(t *testing.T, rngitDir, rnsDir string) (*rnsgit.Node, string) {
	t.Helper()
	srvCfg, err := rnsgit.LoadServerConfig(rngitDir)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := rnsgit.PrepareGitIdentity(filepath.Join(rngitDir, "server_id"))
	if err != nil {
		t.Fatal(err)
	}
	node, err := rnsgit.NewNode(srvCfg, sid)
	if err != nil {
		t.Fatal(err)
	}
	if err := node.Start(rnsDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { node.Stop() })
	return node, node.ReposDestHash()
}

func runGoRngitList(t *testing.T, clientRngitDir, rnsDir, destHex, repo string) string {
	return runGoRngitHelper(t, clientRngitDir, rnsDir, destHex, repo, "", "capabilities\nlist\n\n")
}

func runGoRngitHelper(t *testing.T, clientRngitDir, rnsDir, destHex, repo, workDir, stdin string) string {
	t.Helper()
	if workDir != "" {
		t.Setenv("GIT_DIR", filepath.Join(workDir, ".git"))
		t.Setenv("GIT_WORK_TREE", workDir)
	}
	cfg, err := rnsutil.LoadConfigDir(rnsDir)
	if err != nil {
		t.Fatal(err)
	}
	n, err := node.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	client, err := rnsgit.NewClient(rnsgit.ClientOptions{
		ConfigDir:    clientRngitDir,
		RNSConfigDir: rnsDir,
		DestHex:      destHex,
		Group:        "public",
		Repo:         repo,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.UseTransport(n.Transport(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := client.RunGitHelper(ctx, strings.NewReader(stdin), &out, nil); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func rngitGitCmd(args ...string) *exec.Cmd {
	cmd := exec.Command(args[0], args[1:]...) // #nosec G204 -- test git helper
	cmd.Env = rnsgit.GitCleanEnv()
	return cmd
}

func bareHeadRefSHA(t *testing.T, barePath string) (sha, ref string) {
	t.Helper()
	refOut, err := rngitGitCmd("git", "--git-dir", barePath, "symbolic-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("symbolic-ref: %v %s", err, refOut)
	}
	ref = strings.TrimSpace(strings.TrimPrefix(string(refOut), "ref: "))
	shaOut, err := rngitGitCmd("git", "--git-dir", barePath, "rev-parse", ref).CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse: %v %s", err, shaOut)
	}
	return strings.TrimSpace(string(shaOut)), ref
}

func initEmptyGitWorkdir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := rngitGitCmd("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	return dir
}

func verifyFetchedCommit(t *testing.T, workDir, ref string) {
	t.Helper()
	out, err := rngitGitCmd("git", "-C", workDir, "log", "-1", "--oneline", ref).CombinedOutput()
	if err != nil {
		t.Fatalf("git log %s: %v %s", ref, err, out)
	}
	if !strings.Contains(string(out), "init") {
		t.Fatalf("unexpected log: %s", out)
	}
}

func TestLiveRngitGoList(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	rnsSrv := t.TempDir()
	rnsCli := t.TempDir()
	writeUDPPeerConfig(t, rnsSrv, portA, portB)
	writeUDPPeerConfig(t, rnsCli, portB, portA)

	cfgDir := t.TempDir()
	repoRoot := filepath.Join(cfgDir, "repos", "public")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	rngitDir := filepath.Join(cfgDir, "rngit")
	writeRNGitServerConfig(t, rngitDir, repoRoot)
	demoPath := filepath.Join(repoRoot, "demo")
	initBareRepoWithCommit(t, demoPath)

	_, destHex := startGoRngitNode(t, rngitDir, rnsSrv)
	time.Sleep(500 * time.Millisecond)

	out := runGoRngitList(t, t.TempDir(), rnsCli, destHex, "demo")
	if !strings.Contains(out, "refs/heads/") {
		t.Fatalf("unexpected list: %s", out)
	}
}

func TestLiveGoRngitClientPythonRngitServer(t *testing.T) {
	liveOrSkip(t)
	script := pyScript(t, "rngit_listen.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("rngit_listen.py missing")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	rnsCli := t.TempDir()
	writeUDPPeerConfig(t, rnsCli, portB, portA)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	py := exec.CommandContext(ctx, pythonExe(), script)
	py.Env = append(rnsgit.GitCleanEnv(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(portA),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(portB),
	)
	stdout, err := py.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	py.Stderr = os.Stderr
	if err := py.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = py.Process.Kill() }()

	destHex, err := readReadyLine(t, stdout, 45*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	rngitDir := t.TempDir()
	out := runGoRngitList(t, rngitDir, rnsCli, destHex, "demo")
	if !strings.Contains(out, "refs/heads/") {
		t.Fatalf("unexpected list: %s", out)
	}
}

func TestLivePythonRngitClientGoRngitServer(t *testing.T) {
	liveOrSkip(t)
	script := pyScript(t, "rngit_client.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("rngit_client.py missing")
	}
	if _, err := exec.LookPath("git-remote-rns"); err != nil {
		t.Skip("git-remote-rns not installed")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	rnsSrv := t.TempDir()
	writeUDPPeerConfig(t, rnsSrv, portA, portB)

	cfgDir := t.TempDir()
	repoRoot := filepath.Join(cfgDir, "repos", "public")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	rngitDir := filepath.Join(cfgDir, "rngit")
	writeRNGitServerConfig(t, rngitDir, repoRoot)
	initBareRepoWithCommit(t, filepath.Join(repoRoot, "demo"))

	_, destHex := startGoRngitNode(t, rngitDir, rnsSrv)
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(rnsgit.GitCleanEnv(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(portB),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(portA),
		"INTEROP_IDENTITY_HASH="+destHex,
		"GIT_REMOTE_RNS=git-remote-rns",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python rngit client: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("refs/heads/")) {
		t.Fatalf("missing refs in output: %s", out)
	}
}

func TestLiveRngitGoFetch(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	rnsSrv := t.TempDir()
	rnsCli := t.TempDir()
	writeUDPPeerConfig(t, rnsSrv, portA, portB)
	writeUDPPeerConfig(t, rnsCli, portB, portA)

	cfgDir := t.TempDir()
	repoRoot := filepath.Join(cfgDir, "repos", "public")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	rngitDir := filepath.Join(cfgDir, "rngit")
	writeRNGitServerConfig(t, rngitDir, repoRoot)
	demoPath := filepath.Join(repoRoot, "demo")
	initBareRepoWithCommit(t, demoPath)

	_, destHex := startGoRngitNode(t, rngitDir, rnsSrv)
	time.Sleep(500 * time.Millisecond)

	sha, ref := bareHeadRefSHA(t, demoPath)
	workDir := initEmptyGitWorkdir(t)
	stdin := fmt.Sprintf("capabilities\nfetch %s %s\n\n", sha, ref)
	runGoRngitHelper(t, t.TempDir(), rnsCli, destHex, "demo", workDir, stdin)
	verifyFetchedCommit(t, workDir, ref)
}

func TestLiveGoRngitClientPythonRngitServerFetch(t *testing.T) {
	liveOrSkip(t)
	script := pyScript(t, "rngit_listen.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("rngit_listen.py missing")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	rnsCli := t.TempDir()
	writeUDPPeerConfig(t, rnsCli, portB, portA)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	py := exec.CommandContext(ctx, pythonExe(), script)
	py.Env = append(rnsgit.GitCleanEnv(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(portA),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(portB),
	)
	stdout, err := py.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	py.Stderr = os.Stderr
	if err := py.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = py.Process.Kill() }()

	destHex, err := readReadyLine(t, stdout, 45*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	listOut := runGoRngitList(t, t.TempDir(), rnsCli, destHex, "demo")
	if !strings.Contains(listOut, "refs/heads/") {
		t.Fatalf("unexpected list: %s", listOut)
	}
	sha, ref := bareHeadRefSHAFromList(listOut)
	if sha == "" {
		t.Fatalf("no branch ref in list: %s", listOut)
	}
	workDir := initEmptyGitWorkdir(t)
	stdin := fmt.Sprintf("capabilities\nfetch %s %s\n\n", sha, ref)
	runGoRngitHelper(t, t.TempDir(), rnsCli, destHex, "demo", workDir, stdin)
	verifyFetchedCommit(t, workDir, ref)
}

func TestLivePythonRngitClientGoRngitServerFetch(t *testing.T) {
	liveOrSkip(t)
	script := pyScript(t, "rngit_client.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("rngit_client.py missing")
	}
	if _, err := exec.LookPath("git-remote-rns"); err != nil {
		t.Skip("git-remote-rns not installed")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	rnsSrv := t.TempDir()
	writeUDPPeerConfig(t, rnsSrv, portA, portB)

	cfgDir := t.TempDir()
	repoRoot := filepath.Join(cfgDir, "repos", "public")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	rngitDir := filepath.Join(cfgDir, "rngit")
	writeRNGitServerConfig(t, rngitDir, repoRoot)
	initBareRepoWithCommit(t, filepath.Join(repoRoot, "demo"))

	_, destHex := startGoRngitNode(t, rngitDir, rnsSrv)
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(rnsgit.GitCleanEnv(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(portB),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(portA),
		"INTEROP_IDENTITY_HASH="+destHex,
		"GIT_REMOTE_RNS=git-remote-rns",
		"INTEROP_FETCH=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python rngit fetch client: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("FETCH_OK")) {
		t.Fatalf("fetch failed: %s", out)
	}
}

func bareHeadRefSHAFromList(listOut string) (sha, ref string) {
	for line := range strings.SplitSeq(listOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 || parts[1] == "HEAD" || !strings.HasPrefix(parts[1], "refs/heads/") {
			continue
		}
		return parts[0], parts[1]
	}
	return "", ""
}
