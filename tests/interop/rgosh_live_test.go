// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live rgosh interop: Go↔Go native and Go↔Python rnsh (--compat).
// Set RUN_LIVE_INTEROP=1 to enable. Python tests need RETICULUM_PATH.

package interop

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rgosh"
	"quad4/reticulum-go/pkg/rnsutil"
)

func ensureRgosh(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(repoRoot(t), "bin", "rgosh")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/rgosh")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build rgosh: %v\n%s", err, out)
	}
	return bin
}

func TestLiveGoToGoRgoshPipe(t *testing.T) {
	liveOrSkip(t)

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	cfgDirA := t.TempDir()
	cfgDirB := t.TempDir()
	writeUDPPeerConfig(t, cfgDirA, portA, portB)
	writeUDPPeerConfig(t, cfgDirB, portB, portA)

	cfgA, err := rnsutil.LoadConfigDir(cfgDirA)
	if err != nil {
		t.Fatal(err)
	}
	nA, err := node.New(cfgA)
	if err != nil {
		t.Fatal(err)
	}
	if err := nA.Start(); err != nil {
		t.Fatal(err)
	}
	defer nA.Stop()

	idListen, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(idListen, destination.In, destination.Single, rnsutil.RgoshAppName, nA.Transport())
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)
	dest.SetLinkEstablishedCallback(func(lnk any) {
		l, ok := lnk.(*link.Link)
		if !ok || l == nil {
			return
		}
		ch := l.GetChannel()
		_ = rgosh.RegisterNative(ch)
		sess := rgosh.NewSession(rgosh.Config{
			Listener:   true,
			AllowAll:   true,
			DefaultCmd: []string{"/bin/echo", "live-rgosh-ok"},
		}, rgosh.ChannelSender{Ch: ch})
		sess.StartProcess = rgosh.StartLocalProcess
		sess.OnTeardown = func() { l.Teardown() }
		ch.AddMessageHandler(func(msg rgosh.Message) bool {
			_ = sess.HandleMessage(msg)
			return true
		})
	})
	_ = dest.Announce(false, nil, nil)

	rgoshBin := ensureRgosh(t)
	destHex := hex.EncodeToString(dest.GetHash())
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, rgoshBin, "-config", cfgDirB, "-N", "-m", destHex, "/bin/echo", "live-rgosh-ok")
	cmd.Stdin = bytes.NewReader(nil)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rgosh: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("live-rgosh-ok")) {
		t.Fatalf("stdout missing marker: %s", out)
	}
}

func TestLiveGoToGoRgoshPTY(t *testing.T) {
	liveOrSkip(t)
	TestLiveGoToGoRgoshPipe(t)
}

func TestLiveGoAuthDeny(t *testing.T) {
	liveOrSkip(t)

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	cfgDirA := t.TempDir()
	cfgDirB := t.TempDir()
	writeUDPPeerConfig(t, cfgDirA, portA, portB)
	writeUDPPeerConfig(t, cfgDirB, portB, portA)

	idAllowed, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	rgoshBin := ensureRgosh(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	listen := exec.CommandContext(ctx, rgoshBin, "-config", cfgDirA, "-l", "-a", hex.EncodeToString(idAllowed.Hash()), "/bin/echo", "should-not-run")
	if err := listen.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listen.Process.Kill() }()

	time.Sleep(800 * time.Millisecond)

	idPath := filepath.Join(cfgDirA, "storage", "identities", rnsutil.RgoshAppName)
	id, err := identity.FromFile(idPath)
	if err != nil {
		time.Sleep(500 * time.Millisecond)
		id, err = identity.FromFile(idPath)
		if err != nil {
			t.Fatalf("listener identity: %v", err)
		}
	}
	destHash := destination.Hash(id, rnsutil.RgoshAppName)

	client := exec.CommandContext(ctx, rgoshBin, "-config", cfgDirB, "-m", hex.EncodeToString(destHash), "/bin/echo", "attacker")
	out, _ := client.CombinedOutput()
	if bytes.Contains(out, []byte("should-not-run")) {
		t.Fatalf("auth deny failed: %s", out)
	}
}

func TestLiveGoForcedCommand(t *testing.T) {
	liveOrSkip(t)

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	cfgDirA := t.TempDir()
	cfgDirB := t.TempDir()
	writeUDPPeerConfig(t, cfgDirA, portA, portB)
	writeUDPPeerConfig(t, cfgDirB, portB, portA)

	rgoshBin := ensureRgosh(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	listen := exec.CommandContext(ctx, rgoshBin, "-config", cfgDirA, "-l", "-n", "-C", "/bin/echo", "forced-only")
	if err := listen.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listen.Process.Kill() }()
	time.Sleep(800 * time.Millisecond)

	idPath := filepath.Join(cfgDirA, "storage", "identities", rnsutil.RgoshAppName)
	id, err := identity.FromFile(idPath)
	if err != nil {
		time.Sleep(500 * time.Millisecond)
		id, err = identity.FromFile(idPath)
		if err != nil {
			t.Fatal(err)
		}
	}
	destHash := destination.Hash(id, rnsutil.RgoshAppName)
	client := exec.CommandContext(ctx, rgoshBin, "-config", cfgDirB, "-N", "-m", "-w", "20", hex.EncodeToString(destHash), "/bin/echo", "client-cmd")
	client.Stdin = bytes.NewReader(nil)
	out, err := client.CombinedOutput()
	if !bytes.Contains(out, []byte("forced-only")) {
		t.Fatalf("expected forced command output, err=%v out=%s", err, out)
	}
}

func TestLiveGoCompatToPythonRnsh(t *testing.T) {
	liveOrSkip(t)
	script := filepath.Join(repoRoot(t), "tests", "interop", "py", "rnsh_listen.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("rnsh_listen.py missing")
	}
	if os.Getenv("RETICULUM_PATH") == "" {
		t.Skip("RETICULUM_PATH required for Python rnsh")
	}

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	cfgDirA := t.TempDir()
	cfgDirB := t.TempDir()
	writeUDPPeerConfig(t, cfgDirA, portA, portB)
	writeUDPPeerConfig(t, cfgDirB, portB, portA)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	py := exec.CommandContext(ctx, pythonExe(), script)
	py.Env = append(os.Environ(),
		"INTEROP_CONFIG_DIR="+cfgDirA,
		"INTEROP_LISTEN_PORT="+strconv.Itoa(portA),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(portB),
		"INTEROP_COMMAND=/bin/echo py-rnsh-ok",
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

	destHex, err := readReadyLine(t, stdout, 30*time.Second)
	if err != nil {
		if py.ProcessState != nil && py.ProcessState.ExitCode() == 77 {
			t.Skip("python rnsh unavailable")
		}
		t.Fatal(err)
	}

	rgoshBin := ensureRgosh(t)
	cmd := exec.CommandContext(ctx, rgoshBin, "-config", cfgDirB, "--compat", "-N", "-m", destHex, "/bin/echo", "py-rnsh-ok")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rgosh --compat: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("py-rnsh-ok")) {
		t.Fatalf("missing output: %s", out)
	}
}

func TestLivePythonRnshToGoCompat(t *testing.T) {
	liveOrSkip(t)
	script := filepath.Join(repoRoot(t), "tests", "interop", "py", "rnsh_client.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("rnsh_client.py missing")
	}
	if os.Getenv("RETICULUM_PATH") == "" {
		t.Skip("RETICULUM_PATH required for Python rnsh")
	}

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	cfgDirA := t.TempDir()
	cfgDirB := t.TempDir()
	writeUDPPeerConfig(t, cfgDirA, portA, portB)
	writeUDPPeerConfig(t, cfgDirB, portB, portA)

	rgoshBin := ensureRgosh(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	listen := exec.CommandContext(ctx, rgoshBin, "-config", cfgDirA, "-l", "-n", "--compat", "/bin/echo", "go-compat-ok")
	if err := listen.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listen.Process.Kill() }()
	time.Sleep(800 * time.Millisecond)

	idPath := filepath.Join(cfgDirA, "storage", "identities", rnsutil.RnshAppName)
	id, err := identity.FromFile(idPath)
	if err != nil {
		time.Sleep(500 * time.Millisecond)
		id, err = identity.FromFile(idPath)
		if err != nil {
			t.Fatal(err)
		}
	}
	destHex := hex.EncodeToString(destination.Hash(id, rnsutil.RnshAppName))

	py := exec.CommandContext(ctx, pythonExe(), script)
	py.Env = append(os.Environ(),
		"INTEROP_CONFIG_DIR="+cfgDirB,
		"INTEROP_LISTEN_PORT="+strconv.Itoa(portB),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(portA),
		"INTEROP_DEST_HASH="+destHex,
		"INTEROP_COMMAND=/bin/echo go-compat-ok",
	)
	out, err := py.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 77 {
			t.Skip("python rnsh unavailable")
		}
		t.Fatalf("python rnsh client: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("go-compat-ok")) {
		t.Fatalf("missing output: %s", out)
	}
}
