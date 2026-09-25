// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live rngit page-server, push, and management interop over paired UDP.
// Set RUN_LIVE_INTEROP=1 to enable.

package interop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	rlink "github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/rnsgit"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

// writeRNGitPagesServerConfig writes a server config with the NomadNet page
// destination and stats recording enabled.
func writeRNGitPagesServerConfig(t *testing.T, rngitDir, repoRoot string) {
	t.Helper()
	if err := os.MkdirAll(rngitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rngitDir, "config"), []byte(strings.Join([]string{
		"[repositories]",
		"public = " + repoRoot,
		"[access]",
		"public = rw:all, s:all",
		"[rngit]",
		"announce_interval = 0",
		"mirror_interval = 0",
		"record_stats = yes",
		"node_name = Interop Go Node",
		"[pages]",
		"serve_nomadnet = yes",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
}

// startGoRngitPagesNode starts a Go rngit node with the page destination.
func startGoRngitPagesNode(t *testing.T, rngitDir, rnsDir string) (*rnsgit.Node, string, string) {
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
	return node, node.ReposDestHash(), node.PageDestHash()
}

// runPythonPageRequestToGo drives rngit_page_client.py against a
// nomadnetwork.node destination and asserts REQUEST_OK.
func runPythonPageRequestToGo(t *testing.T, ctx context.Context, pyListen, pyForward int,
	destHex, reqPath string, vars map[string]string, expectContains string) {
	t.Helper()
	script := pyScript(t, "rngit_page_client.py")
	varsJSON, err := json.Marshal(vars)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(rnsgit.GitCleanEnv(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_DEST_HASH="+destHex,
		"INTEROP_REQUEST_PATH="+reqPath,
		"INTEROP_PAGE_VARS="+string(varsJSON),
		"INTEROP_EXPECT_CONTAINS="+expectContains,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python page client %s: %v\n%s", reqPath, err, out)
	}
	if !bytes.Contains(out, []byte("REQUEST_OK")) {
		t.Fatalf("page request %s missing REQUEST_OK: %s", reqPath, out)
	}
	time.Sleep(300 * time.Millisecond)
}

// TestLiveRngitGoPagesPythonClient runs the Go page server and has a Python
// NomadNet-style client load the main page routes over real links.
func TestLiveRngitGoPagesPythonClient(t *testing.T) {
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
	writeRNGitPagesServerConfig(t, rngitDir, repoRoot)
	initBareRepoWithCommit(t, filepath.Join(repoRoot, "demo"))

	_, _, pageDestHex := startGoRngitPagesNode(t, rngitDir, rnsSrv)
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	repoVars := map[string]string{"g": "public", "r": "demo"}
	checks := []struct {
		path    string
		vars    map[string]string
		expects string
	}{
		{"/page/index.mu", nil, "public"},
		{"/page/group.mu", map[string]string{"g": "public"}, "demo"},
		{"/page/repo.mu", repoVars, "rns://"},
		{"/page/tree.mu", repoVars, "README"},
		{"/page/commits.mu", repoVars, "init"},
		{"/page/refs.mu", repoVars, "main"},
		{"/page/stats.mu", repoVars, "ctivity"},
		{"/page/releases.mu", repoVars, "elease"},
		{"/page/work.mu", repoVars, "ork"},
	}
	for _, c := range checks {
		runPythonPageRequestToGo(t, ctx, portB, portA, pageDestHex, c.path, c.vars, c.expects)
	}
}

// readReadyAndPagesLines scans the listener output for the READY <hash> and
// PAGES <hash> lines and returns both destination hex strings.
func readReadyAndPagesLines(t *testing.T, r io.Reader, timeout time.Duration) (string, string, error) {
	t.Helper()
	type result struct {
		ready, pages string
		err          error
	}
	ch := make(chan result, 1)
	go func() {
		var res result
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if after, ok := strings.CutPrefix(line, "READY "); ok {
				res.ready = strings.TrimSpace(after)
			}
			if after, ok := strings.CutPrefix(line, "PAGES "); ok {
				res.pages = strings.TrimSpace(after)
			}
			if res.ready != "" && res.pages != "" {
				ch <- res
				return
			}
		}
		res.err = sc.Err()
		if res.err == nil {
			res.err = fmt.Errorf("listener exited before PAGES (ready=%q)", res.ready)
		}
		ch <- res
	}()
	select {
	case res := <-ch:
		return res.ready, res.pages, res.err
	case <-time.After(timeout):
		return "", "", context.DeadlineExceeded
	}
}

// goPageRequest opens a link to a nomadnetwork.node destination and issues a
// single request with var_* fields, returning the response body.
func goPageRequest(t *testing.T, ctx context.Context, tr *transport.Transport, iface *interfaces.UDPInterface,
	destHash []byte, path string, vars map[string]string) []byte {
	t.Helper()
	if err := waitPath(ctx, tr, destHash, 40*time.Second); err != nil {
		t.Fatalf("path to python pages: %v", err)
	}
	srvID, err := identity.Recall(destHash)
	if err != nil {
		t.Fatalf("recall python identity: %v", err)
	}
	destOut, err := destination.FromHash(destHash, srvID, destination.Single, tr)
	if err != nil {
		t.Fatalf("from hash: %v", err)
	}
	established := make(chan struct{})
	lnk := rlink.NewLink(destOut, tr, iface, func(_ *rlink.Link) {
		close(established)
	}, nil)
	defer lnk.Teardown()
	if err := lnk.Establish(); err != nil {
		t.Fatalf("establish: %v", err)
	}
	select {
	case <-established:
	case <-time.After(45 * time.Second):
		t.Fatal("link establish timeout")
	}
	lnk.Start()

	fields := map[string]any{}
	for k, v := range vars {
		fields["var_"+k] = v
	}
	done := make(chan []byte, 1)
	fail := make(chan struct{}, 1)
	receipt, err := lnk.Request(path, fields, 60*time.Second)
	if err != nil {
		t.Fatalf("request %s: %v", path, err)
	}
	receipt.SetResponseCallback(func(r *rlink.RequestReceipt) {
		done <- append([]byte(nil), r.GetResponse()...)
	})
	receipt.SetFailedCallback(func(_ *rlink.RequestReceipt) {
		select {
		case fail <- struct{}{}:
		default:
		}
	})
	select {
	case got := <-done:
		return got
	case <-fail:
		t.Fatalf("request %s failed", path)
	case <-ctx.Done():
		t.Fatalf("request %s: %v", path, ctx.Err())
	}
	return nil
}

// TestLiveRngitPythonPagesGoClient runs the Python rngit node with
// serve_nomadnet and has the Go side load pages over a real link.
func TestLiveRngitPythonPagesGoClient(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	py := exec.CommandContext(ctx, pythonExe(), script)
	py.Env = append(rnsgit.GitCleanEnv(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(portA),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(portB),
		"INTEROP_PAGES=1",
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

	// Read READY and PAGES in one scan pass. Separate scanners would let the
	// first read ahead and swallow the second line.
	destHex, pageDestHex, err := readReadyAndPagesLines(t, stdout, 45*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	pageHash, err := hex.DecodeString(pageDestHex)
	if err != nil || len(pageHash) != 16 {
		t.Fatalf("bad page dest hash %q", pageDestHex)
	}
	_ = destHex
	time.Sleep(500 * time.Millisecond)

	tr, iface, cleanup := setupGoUDPPeer(t, portA, portB)
	defer cleanup()

	repoVars := map[string]string{"g": "public", "r": "demo"}
	checks := []struct {
		path    string
		vars    map[string]string
		expects string
	}{
		{"/page/index.mu", nil, "public"},
		{"/page/group.mu", map[string]string{"g": "public"}, "demo"},
		{"/page/repo.mu", repoVars, "rns://"},
		{"/page/tree.mu", repoVars, "README"},
		{"/page/commits.mu", repoVars, "init"},
		{"/page/refs.mu", repoVars, "main"},
	}
	for _, c := range checks {
		got := goPageRequest(t, ctx, tr, iface, pageHash, c.path, c.vars)
		if !bytes.Contains(got, []byte(c.expects)) {
			t.Fatalf("page %s missing %q:\n%s", c.path, c.expects, got)
		}
	}
}
