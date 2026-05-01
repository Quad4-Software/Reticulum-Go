// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bufio"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
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

func runPythonPageRequest(
	t *testing.T,
	ctx context.Context,
	pyListen, pyForward int,
	goDestHash []byte,
	reqPath, expectContains string,
) {
	t.Helper()

	script := pyScript(t, "pageserver_client.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_GO_DEST_HASH="+hex.EncodeToString(goDestHash),
		"INTEROP_REQUEST_PATH="+reqPath,
		"INTEROP_EXPECT_CONTAINS="+expectContains,
	)
	cmd.Stderr = os.Stderr

	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python client: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 20*time.Second)
	if err != nil {
		t.Fatalf("wait READY: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(line), "READY") {
		t.Fatalf("expected READY, got %q", line)
	}

	line, err = readLineTimeout(ctx, br, 75*time.Second)
	if err != nil {
		t.Fatalf("wait REQUEST_OK for %s: %v", reqPath, err)
	}
	if strings.TrimSpace(line) != "REQUEST_OK" {
		t.Fatalf("expected REQUEST_OK for %s, got %q", reqPath, line)
	}
}

func TestLiveInteropPythonNomadNetPageServerRequests(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	goListen := freeUDPPort(t)
	pyListen := freeUDPPort(t)

	pageServerHome := t.TempDir()
	goDestHash := preparePageServerIdentity(t, pageServerHome)

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

	cmd := exec.CommandContext(
		ctx,
		filepath.Join(pageServerDir, "example-pageserver"),
		"-udp",
		"-listen-port",
		strconv.Itoa(goListen),
		"-target-port",
		strconv.Itoa(pyListen),
		"-log-level",
		"7",
	)
	cmd.Dir = pageServerDir
	cmd.Env = append(os.Environ(), "HOME="+pageServerHome)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start pageserver: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	time.Sleep(4 * time.Second)

	runPythonPageRequest(t, ctx, pyListen, goListen, goDestHash, "/page/index.mu", "Reticulum-Go Page Node")
	runPythonPageRequest(t, ctx, pyListen, goListen, goDestHash, "/file/interop_test_file.txt", "PY_FILE_TEST")
}

func TestLiveInteropPythonPageServerLargeFileRequest(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	goListen := freeUDPPort(t)
	pyListen := freeUDPPort(t)

	pageServerHome := t.TempDir()
	goDestHash := preparePageServerIdentity(t, pageServerHome)

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

	cmd := exec.CommandContext(
		ctx,
		filepath.Join(pageServerDir, "example-pageserver"),
		"-udp",
		"-listen-port",
		strconv.Itoa(goListen),
		"-target-port",
		strconv.Itoa(pyListen),
		"-log-level",
		"7",
	)
	cmd.Dir = pageServerDir
	cmd.Env = append(os.Environ(), "HOME="+pageServerHome)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start pageserver: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	time.Sleep(4 * time.Second)

	runPythonPageRequest(t, ctx, pyListen, goListen, goDestHash, "/file/interop_large_file.txt", "LARGE_INTEROP_MARKER")
}

func TestLiveInteropPythonPageServerLargePageRequest(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	goListen := freeUDPPort(t)
	pyListen := freeUDPPort(t)

	pageServerHome := t.TempDir()
	goDestHash := preparePageServerIdentity(t, pageServerHome)

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

	cmd := exec.CommandContext(
		ctx,
		filepath.Join(pageServerDir, "example-pageserver"),
		"-udp",
		"-listen-port",
		strconv.Itoa(goListen),
		"-target-port",
		strconv.Itoa(pyListen),
		"-log-level",
		"7",
	)
	cmd.Dir = pageServerDir
	cmd.Env = append(os.Environ(), "HOME="+pageServerHome)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start pageserver: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	time.Sleep(4 * time.Second)

	runPythonPageRequest(t, ctx, pyListen, goListen, goDestHash, "/page/interop_large_page.mu", "LARGE_PAGE_INTEROP_MARKER")
}
