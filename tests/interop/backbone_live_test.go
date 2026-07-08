// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bufio"
	"context"
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/transport"
)

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestLiveInteropBackboneHDLCEchoPython(t *testing.T) {
	liveOrSkip(t)

	port := freeTCPPort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	script := pyScript(t, "backbone_hdlc_echo.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_PORT="+strconv.Itoa(port),
		"INTEROP_MODE=server",
	)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 10*time.Second)
	if err != nil {
		t.Fatalf("READY: %v", err)
	}
	if strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q", line)
	}

	backbone.Shutdown()
	hub, err := backbone.Init(backbone.DefaultBackend())
	if err != nil {
		t.Fatal(err)
	}
	defer backbone.Shutdown()

	client, err := interfaces.NewBackboneClientInterface("go_client", &common.InterfaceConfig{
		Enabled:    true,
		TargetHost: "127.0.0.1",
		TargetPort: port,
	}, hub)
	if err != nil {
		t.Fatal(err)
	}

	var got []byte
	client.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got = append([]byte(nil), data...)
	})
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	defer client.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if client.IsOnline() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !client.IsOnline() {
		t.Fatal("client offline")
	}

	payload := []byte{0x42, 0x43, 0x44}
	if err := client.Send(payload, ""); err != nil {
		t.Fatal(err)
	}

	for time.Now().Before(deadline) {
		if len(got) == len(payload) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if string(got) != string(payload) {
		t.Fatalf("echo mismatch: got %x want %x", got, payload)
	}
}

func TestLiveInteropBackboneGoServerPythonHDLC(t *testing.T) {
	liveOrSkip(t)

	backbone.Shutdown()
	hub, err := backbone.Init(backbone.DefaultBackend())
	if err != nil {
		t.Fatal(err)
	}
	defer backbone.Shutdown()

	port := freeTCPPort(t)
	server, err := interfaces.NewBackboneInterface("go_server", &common.InterfaceConfig{
		Enabled: true,
		Address: "127.0.0.1",
		Port:    port,
	}, hub, func(c *interfaces.BackboneClientInterface) {
		c.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
			_ = c.Send(data, "")
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	script := pyScript(t, "backbone_hdlc_echo.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_MODE=client",
		"INTEROP_TARGET_PORT="+strconv.Itoa(port),
	)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 10*time.Second)
	if err != nil {
		t.Fatalf("python echo: %v", err)
	}
	got, err := hex.DecodeString(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("bad hex: %q", line)
	}
	want := []byte{0x42, 0x43, 0x44}
	if string(got) != string(want) {
		t.Fatalf("got %x want %x", got, want)
	}
}

func TestLiveInteropBackboneRNSAnnounce(t *testing.T) {
	liveOrSkip(t)
	if runtime.GOOS != "linux" {
		t.Skip("Python BackboneInterface requires Linux")
	}

	port := freeTCPPort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	script := pyScript(t, "backbone_server.py")
	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(), "INTEROP_PORT="+strconv.Itoa(port))
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python backbone: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 30*time.Second)
	if err != nil {
		t.Fatalf("READY: %v", err)
	}
	if strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q", line)
	}
	hashLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	pyHash, err := hex.DecodeString(strings.TrimSpace(hashLine))
	if err != nil || len(pyHash) != 16 {
		t.Fatalf("bad hash %q: %v", hashLine, err)
	}

	backbone.Shutdown()
	hub, err := backbone.Init(backbone.DefaultBackend())
	if err != nil {
		t.Fatal(err)
	}
	defer backbone.Shutdown()

	cfg := &common.ReticulumConfig{}
	tr := transport.NewTransport(cfg)
	iface, err := interfaces.NewBackboneClientInterface("interop_bb", &common.InterfaceConfig{
		Enabled:    true,
		TargetHost: "127.0.0.1",
		TargetPort: port,
	}, hub)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface("interop_bb", iface); err != nil {
		t.Fatal(err)
	}
	if err := iface.Start(); err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatal(err)
	}

	if err := waitPath(ctx, tr, pyHash, 60*time.Second); err != nil {
		t.Fatalf("wait path: %v", err)
	}
	if !tr.HasPath(pyHash) {
		t.Fatal("HasPath false after backbone announce")
	}
}
