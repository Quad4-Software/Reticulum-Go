// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/reticulumconfig"
	"quad4/reticulum-go/pkg/transport"
	"quad4/reticulum-go/tests/interop/harness"
)

func preparePageServerIdentity(t *testing.T, homeDir string) []byte {
	t.Helper()

	idPath := filepath.Join(homeDir, ".reticulum-go", "storage", "identity")
	if err := os.MkdirAll(filepath.Dir(idPath), 0o700); err != nil {
		t.Fatalf("mkdir identity dir: %v", err)
	}

	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("new identity: %v", err)
	}
	if err := id.ToFile(idPath); err != nil {
		t.Fatalf("write identity: %v", err)
	}

	tr := transport.NewTransport(common.DefaultConfig())
	dest, err := destination.New(id, destination.In, destination.Single, "nomadnetwork", tr, "node")
	if err != nil {
		t.Fatalf("destination hash derive: %v", err)
	}

	return dest.GetHash()
}

func writePageServerInteropReticulumConfig(t *testing.T, home string, goListen, pyListen int) {
	t.Helper()
	cfgPath := filepath.Join(home, ".reticulum-go", "config")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := reticulumconfig.DefaultConfig()
	cfg.ConfigPath = cfgPath
	cfg.EnableTransport = true
	cfg.ShareInstance = false
	cfg.EnableSandbox = false
	cfg.Interfaces = map[string]*common.InterfaceConfig{
		"UDP": {
			Name:       "UDP",
			Type:       "UDPInterface",
			Enabled:    true,
			Address:    fmt.Sprintf("0.0.0.0:%d", goListen),
			TargetHost: fmt.Sprintf("127.0.0.1:%d", pyListen),
		},
	}
	if err := reticulumconfig.SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func runPythonPageRequest(
	t *testing.T,
	ctx context.Context,
	sess *harness.Session,
	pyListen, pyForward int,
	goDestHash []byte,
	reqPath, expectContains string,
) {
	t.Helper()

	probe := harness.StartPython(t, harness.ProbeOpts{
		Ctx:          ctx,
		Script:       pyScript(t, "pageserver_client.py"),
		Events:       sess.Events,
		ArtifactsDir: sess.Dir,
		Env: []string{
			"INTEROP_LISTEN_PORT=" + strconv.Itoa(pyListen),
			"INTEROP_FORWARD_PORT=" + strconv.Itoa(pyForward),
			"INTEROP_GO_DEST_HASH=" + hex.EncodeToString(goDestHash),
			"INTEROP_REQUEST_PATH=" + reqPath,
			"INTEROP_EXPECT_CONTAINS=" + expectContains,
		},
	})

	probe.WaitExact(t, ctx, "READY", 20*time.Second, harness.KindReady)
	probe.WaitExact(t, ctx, "REQUEST_OK", 120*time.Second, harness.KindRequest)
	sess.Emit("request_ok", harness.KindRequest, reqPath)
	select {
	case <-probe.Done():
	case <-ctx.Done():
		probe.Kill(3 * time.Second)
		t.Fatalf("pageserver client did not exit: %v", ctx.Err())
	case <-time.After(10 * time.Second):
		probe.Kill(3 * time.Second)
		t.Fatal("pageserver client did not exit after REQUEST_OK")
	}
	// Let the UDP listen port fully release before a follow-up client binds it.
	time.Sleep(200 * time.Millisecond)
}

func startPageServerBinary(t *testing.T, ctx context.Context, home, pageServerDir string) *exec.Cmd {
	t.Helper()
	bin := filepath.Join(pageServerDir, "example-pageserver")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("pageserver binary missing at %s (build examples/pageserver first): %v", bin, err)
	}
	cmd := exec.CommandContext(ctx, bin, "-log-level", "7")
	cmd.Dir = pageServerDir
	cmd.Env = append(os.Environ(), "HOME="+home)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start pageserver: %v", err)
	}
	return cmd
}

func TestLiveInteropPythonNomadNetPageServerRequests(t *testing.T) {
	liveOrSkip(t)
	sess := harness.Begin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	goListen := freeUDPPort(t)
	pyListen := freeUDPPort(t)

	pageServerHome := t.TempDir()
	goDestHash := preparePageServerIdentity(t, pageServerHome)
	writePageServerInteropReticulumConfig(t, pageServerHome, goListen, pyListen)

	pageServerDir := filepath.Join(scriptDir(t), "..", "..", "examples", "pageserver")
	filesDir := filepath.Join(pageServerDir, "files")
	if err := os.MkdirAll(filesDir, 0o750); err != nil {
		t.Fatalf("mkdir files: %v", err)
	}
	testFilePath := filepath.Join(filesDir, "interop_test_file.txt")
	if err := os.WriteFile(testFilePath, []byte("PY_FILE_TEST\n"), 0o640); err != nil {
		t.Fatalf("write interop test file: %v", err)
	}
	defer func() { _ = os.Remove(testFilePath) }()

	cmd := startPageServerBinary(t, ctx, pageServerHome, pageServerDir)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	time.Sleep(6 * time.Second)

	runPythonPageRequest(t, ctx, sess, pyListen, goListen, goDestHash, "/page/index.mu", "librns via Reticulum-Go")
	runPythonPageRequest(t, ctx, sess, pyListen, goListen, goDestHash, "/file/interop_test_file.txt", "PY_FILE_TEST")
}

func TestLiveInteropPythonPageServerLargeFileRequest(t *testing.T) {
	liveOrSkip(t)
	sess := harness.Begin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	goListen := freeUDPPort(t)
	pyListen := freeUDPPort(t)

	pageServerHome := t.TempDir()
	goDestHash := preparePageServerIdentity(t, pageServerHome)
	writePageServerInteropReticulumConfig(t, pageServerHome, goListen, pyListen)

	pageServerDir := filepath.Join(scriptDir(t), "..", "..", "examples", "pageserver")
	filesDir := filepath.Join(pageServerDir, "files")
	if err := os.MkdirAll(filesDir, 0o750); err != nil {
		t.Fatalf("mkdir files: %v", err)
	}

	largePath := filepath.Join(filesDir, "interop_large_file.txt")
	largeContent := strings.Repeat("x", 18000) + "\nLARGE_INTEROP_MARKER\n" + strings.Repeat("y", 18000) + "\n"
	if err := os.WriteFile(largePath, []byte(largeContent), 0o640); err != nil {
		t.Fatalf("write large interop file: %v", err)
	}
	defer func() { _ = os.Remove(largePath) }()

	cmd := startPageServerBinary(t, ctx, pageServerHome, pageServerDir)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	time.Sleep(6 * time.Second)

	runPythonPageRequest(t, ctx, sess, pyListen, goListen, goDestHash, "/file/interop_large_file.txt", "LARGE_INTEROP_MARKER")
}

func TestLiveInteropPythonPageServerLargePageRequest(t *testing.T) {
	liveOrSkip(t)
	sess := harness.Begin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	goListen := freeUDPPort(t)
	pyListen := freeUDPPort(t)

	pageServerHome := t.TempDir()
	goDestHash := preparePageServerIdentity(t, pageServerHome)
	writePageServerInteropReticulumConfig(t, pageServerHome, goListen, pyListen)

	pageServerDir := filepath.Join(scriptDir(t), "..", "..", "examples", "pageserver")
	pagesDir := filepath.Join(pageServerDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o750); err != nil {
		t.Fatalf("mkdir pages: %v", err)
	}

	largePath := filepath.Join(pagesDir, "interop_large_page.mu")
	largeContent := strings.Repeat("A", 18000) + "\nLARGE_PAGE_INTEROP_MARKER\n" + strings.Repeat("B", 18000) + "\n"
	if err := os.WriteFile(largePath, []byte(largeContent), 0o640); err != nil {
		t.Fatalf("write large interop page: %v", err)
	}
	defer func() { _ = os.Remove(largePath) }()

	cmd := startPageServerBinary(t, ctx, pageServerHome, pageServerDir)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	time.Sleep(6 * time.Second)

	runPythonPageRequest(t, ctx, sess, pyListen, goListen, goDestHash, "/page/interop_large_page.mu", "LARGE_PAGE_INTEROP_MARKER")
}
