// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live rngit push and management interop over paired UDP.
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

	"github.com/Quad4-Software/Reticulum-Go/pkg/rnsgit"
)

// gitWorkRepoWithCommit creates a non-bare repo with one commit on main.
func gitWorkRepoWithCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	env := append(rnsgit.GitCleanEnv(),
		"GIT_AUTHOR_NAME=interop",
		"GIT_AUTHOR_EMAIL=interop@test",
		"GIT_COMMITTER_NAME=interop",
		"GIT_COMMITTER_EMAIL=interop@test",
	)
	run := func(args ...string) {
		cmd := exec.Command("git", args...) // #nosec G204 -- test git
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "pushed.txt"), []byte("push interop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "pushed.txt")
	run("-c", "commit.gpgsign=false", "commit", "-m", "push interop")
	return dir
}

// TestLiveRngitGoPush pushes a new branch from the Go helper to a Go node.
func TestLiveRngitGoPush(t *testing.T) {
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

	workDir := gitWorkRepoWithCommit(t)
	out := runGoRngitHelper(t, t.TempDir(), rnsCli, destHex, "demo", workDir,
		"capabilities\nlist for-push\npush refs/heads/main:refs/heads/interop-go\n\n")
	if !strings.Contains(out, "ok refs/heads/interop-go") {
		t.Fatalf("push failed: %s", out)
	}
	shaOut, err := rngitGitCmd("git", "--git-dir", demoPath, "rev-parse", "refs/heads/interop-go").CombinedOutput()
	if err != nil {
		t.Fatalf("remote ref missing after push: %v %s", err, shaOut)
	}
}

// TestLiveRngitGoPushToPython pushes a new branch from the Go helper to a
// Python rngit node.
func TestLiveRngitGoPushToPython(t *testing.T) {
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
	rngitDir := t.TempDir()
	repoRoot := filepath.Join(rngitDir, "repos", "public")
	writeUDPPeerConfig(t, rnsCli, portB, portA)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	py := exec.CommandContext(ctx, pythonExe(), script)
	py.Env = append(rnsgit.GitCleanEnv(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(portA),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(portB),
		"INTEROP_RNGIT_DIR="+rngitDir,
		"INTEROP_REPO_ROOT="+repoRoot,
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

	workDir := gitWorkRepoWithCommit(t)
	out := runGoRngitHelper(t, t.TempDir(), rnsCli, destHex, "demo", workDir,
		"capabilities\nlist for-push\npush refs/heads/main:refs/heads/interop-go\n\n")
	if !strings.Contains(out, "ok refs/heads/interop-go") {
		t.Fatalf("push to python failed: %s", out)
	}
}

// TestLivePythonRngitClientGoRngitServerPush pushes a branch from the Python
// git-remote-rns helper to a Go node.
func TestLivePythonRngitClientGoRngitServerPush(t *testing.T) {
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
	demoPath := filepath.Join(repoRoot, "demo")
	initBareRepoWithCommit(t, demoPath)

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
		"INTEROP_PUSH=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python rngit push client: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("PUSH_OK")) {
		t.Fatalf("push failed: %s", out)
	}
	if _, err := rngitGitCmd("git", "--git-dir", demoPath, "rev-parse", "refs/heads/interop-py").CombinedOutput(); err != nil {
		t.Fatal("pushed ref missing on Go server")
	}
}

// TestLivePythonRngitMgmtGoServer issues management requests from Python to
// the Go node: release list, group permission get, work list.
func TestLivePythonRngitMgmtGoServer(t *testing.T) {
	liveOrSkip(t)
	script := pyScript(t, "rngit_mgmt_client.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("rngit_mgmt_client.py missing")
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
	// The permission management check needs group admin, so grant it here.
	writeRNGitServerConfigAccess(t, rngitDir, repoRoot, "rw:all, adm:all")
	initBareRepoWithCommit(t, filepath.Join(repoRoot, "demo"))

	_, destHex := startGoRngitNode(t, rngitDir, rnsSrv)
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(rnsgit.GitCleanEnv(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(portB),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(portA),
		"INTEROP_DEST_HASH="+destHex,
		"INTEROP_REPO_PATH=public/demo",
		"INTEROP_GROUP=public",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python mgmt client: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("MGMT_OK")) {
		t.Fatalf("mgmt checks failed: %s", out)
	}
}

// TestLiveGoRngitMgmtPythonServer drives the Go MgmtClient release list and
// permission get against a Python rngit node.
func TestLiveGoRngitMgmtPythonServer(t *testing.T) {
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
	rngitDir := t.TempDir()
	repoRoot := filepath.Join(rngitDir, "repos", "public")
	writeUDPPeerConfig(t, rnsCli, portB, portA)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	py := exec.CommandContext(ctx, pythonExe(), script)
	py.Env = append(rnsgit.GitCleanEnv(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(portA),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(portB),
		"INTEROP_RNGIT_DIR="+rngitDir,
		"INTEROP_REPO_ROOT="+repoRoot,
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

	clientRngit := t.TempDir()
	mc, err := rnsgit.NewMgmtClient(clientRngit, rnsCli)
	if err != nil {
		t.Fatal(err)
	}
	remote := fmt.Sprintf("rns://%s/public/demo", destHex)
	if err := mc.Connect(ctx, remote); err != nil {
		t.Fatalf("mgmt connect: %v", err)
	}
	if _, err := mc.ReleaseList(ctx, remote); err != nil {
		t.Fatalf("release list: %v", err)
	}
}
