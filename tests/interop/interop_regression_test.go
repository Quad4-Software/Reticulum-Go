// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNomadNetPageExpectMatchesIndexMu keeps the Python pageserver live expect
// string aligned with examples/pageserver/pages/index.mu (previously expected
// "Reticulum-Go Node" while the page only contained "librns via Reticulum-Go").
func TestNomadNetPageExpectMatchesIndexMu(t *testing.T) {
	root := repoRoot(t)
	pagePath := filepath.Join(root, "examples", "pageserver", "pages", "index.mu")
	page, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatal(err)
	}
	const expect = "librns via Reticulum-Go"
	if !bytes.Contains(page, []byte(expect)) {
		t.Fatalf("%s missing %q", pagePath, expect)
	}
	livePath := filepath.Join(root, "tests", "interop", "pageserver_live_test.go")
	live, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatal(err)
	}
	needle := `"/page/index.mu", "` + expect + `"`
	if !bytes.Contains(live, []byte(needle)) {
		t.Fatalf("%s must pass expect %q for /page/index.mu", livePath, expect)
	}
}

// TestPageRequestHelperWaitsForClientExit locks the double-request UDP bind fix:
// the second pageserver_client must not start until the first has exited.
func TestPageRequestHelperWaitsForClientExit(t *testing.T) {
	livePath := filepath.Join(repoRoot(t), "tests", "interop", "pageserver_live_test.go")
	live, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(live, []byte("probe.Done()")) {
		t.Fatal("runPythonPageRequest must wait on probe.Done() before returning")
	}
}

func TestWaitStdoutTokenSkipsNoise(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	go func() {
		_, _ = w.WriteString("[Error] SAM API went offline\n")
		_, _ = w.WriteString("ONLINE\n")
		_ = w.Close()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := waitStdoutToken(ctx, bufio.NewReader(r), "ONLINE", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ONLINE" {
		t.Fatalf("got %q", got)
	}
}

func TestWaitStdoutTokenPrefix(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	go func() {
		_, _ = w.WriteString("noise\n")
		_, _ = w.WriteString("B32=abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst\n")
		_ = w.Close()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := waitStdoutToken(ctx, bufio.NewReader(r), "B32=", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "B32=") {
		t.Fatalf("got %q", got)
	}
}

// TestI2PPeerWriteConfigCreatesParentDir locks the FileNotFoundError fix when
// INTEROP_CONFIG_DIR points at a nested path that does not exist yet.
func TestI2PPeerWriteConfigCreatesParentDir(t *testing.T) {
	script := filepath.Join(repoRoot(t), "tests", "interop", "py", "i2p_peer.py")
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("os.makedirs(cfg_dir, exist_ok=True)")) {
		t.Fatal("i2p_peer.write_config must os.makedirs(cfg_dir) before writing config")
	}
	if !bytes.Contains(body, []byte("RNS.logdest = RNS.LOG_FILE")) {
		t.Fatal("i2p_peer must log to file so READY/ONLINE tokens are not mixed with RNS logs")
	}
}

func TestI2PLiveTestsCreateConfigDirs(t *testing.T) {
	path := filepath.Join(repoRoot(t), "tests", "interop", "i2p_live_test.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`os.MkdirAll(pyCfg`)) {
		t.Fatal("i2p live tests must MkdirAll Python config dirs before spawn")
	}
	if !bytes.Contains(body, []byte("stopPythonI2P")) {
		t.Fatal("i2p live tests must soft-stop Python peers (avoid SAM thrash on Kill)")
	}
}

func TestCLIInteropHelpersAlwaysRebuildBins(t *testing.T) {
	rnxPath := filepath.Join(repoRoot(t), "tests", "interop", "rnx_live_test.go")
	rnx, err := os.ReadFile(rnxPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rnx, []byte("func ensureRgox")) {
		t.Fatal("missing ensureRgox")
	}
	// Stale bin/rgox that forced ShareInstance=true broke UDP interop. ensureRgox
	// must always rebuild, not only when the binary is missing.
	if bytes.Contains(rnx, []byte("if _, err := os.Stat(bin); err != nil")) {
		t.Fatal("ensureRgox must always go build (not Stat-gated)")
	}
	cpPath := filepath.Join(repoRoot(t), "tests", "interop", "path_cp_live_test.go")
	cp, err := os.ReadFile(cpPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(cp, []byte(`build := exec.Command("go", "build", "-o", rgocp`)) {
		t.Fatal("rgocp live test must always go build before use")
	}
}
