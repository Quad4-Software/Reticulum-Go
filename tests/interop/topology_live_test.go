// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live check for rgostatus -T (topology): a Python peer announces over UDP to
// a Go shared instance, then the grouped path view is asserted through the
// real CLI path. Set RUN_LIVE_INTEROP=1.

package interop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/cli"
	"github.com/Quad4-Software/Reticulum-Go/pkg/node"
	"github.com/Quad4-Software/Reticulum-Go/pkg/rnsutil"
)

func TestLiveRgostatusTopology(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	port := freeUDPPort(t)
	ctrlPort := port + 1
	ifacePort := freeUDPPort(t)
	pyListen := freeUDPPort(t)

	cfgDir := t.TempDir()
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
		"[interfaces]",
		"  [[topo_udp]]",
		"    type = UDPInterface",
		"    enabled = yes",
		"    listen_ip = 127.0.0.1",
		"    listen_port = " + strconv.Itoa(ifacePort),
		"    forward_ip = 127.0.0.1",
		"    forward_port = " + strconv.Itoa(pyListen),
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	goCfg, err := rnsutil.LoadConfigDir(cfgDir)
	if err != nil {
		t.Fatalf("load config dir: %v", err)
	}
	n, err := node.New(goCfg)
	if err != nil {
		t.Fatalf("node new: %v", err)
	}
	if err := n.Start(); err != nil {
		t.Fatalf("node start: %v", err)
	}
	defer n.Stop()

	// Python announces toward the Go UDP listen port.
	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "announce_peer.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(ifacePort),
	)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 25*time.Second)
	if err != nil {
		t.Fatalf("wait READY: %v", err)
	}
	if strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q", line)
	}
	hashLine, err := readLineTimeout(ctx, br, 5*time.Second)
	if err != nil {
		t.Fatalf("wait hash: %v", err)
	}
	pyHash := strings.TrimSpace(hashLine)
	if len(pyHash) != 32 {
		t.Fatalf("bad python destination hash: %q", pyHash)
	}

	// Wait for the path table to learn the python destination over the UDP iface.
	client, err := rnsutil.DialRPC(goCfg, nil)
	if err != nil {
		t.Fatalf("rpc dial: %v", err)
	}
	deadline := time.Now().Add(40 * time.Second)
	seen := false
	for time.Now().Before(deadline) {
		paths, err := client.GetPathTable(nil)
		if err == nil {
			for _, p := range paths {
				if p.Interface == "topo_udp" && strings.EqualFold(hex.EncodeToString(p.Hash), pyHash) {
					seen = true
				}
			}
		}
		if seen {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !seen {
		t.Fatal("path table never learned the python announce")
	}

	// Drive the real CLI.
	var buf bytes.Buffer
	code := cli.RunStatus([]string{"-config", cfgDir, "-T"}, cli.Options{Stdout: &buf, Stderr: &buf})
	if code != 0 {
		t.Fatalf("rgostatus -T: code=%d out=%q", code, buf.String())
	}
	got := buf.String()
	for _, want := range []string{"Topology:", "topo_udp", "path", "hops"} {
		if !strings.Contains(got, want) {
			t.Fatalf("topology output missing %q:\n%s", want, got)
		}
	}
	// PrettyHex truncates to 6+6 hex around an ellipsis.
	if !strings.Contains(got, strings.ToLower(pyHash)[:6]) {
		t.Fatalf("topology output missing python destination hash %s:\n%s", pyHash, got)
	}
}
