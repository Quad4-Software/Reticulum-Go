// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live utility interop against Python rnsd / rnid and Go shared instance.
// Set RUN_LIVE_INTEROP=1 to enable.

package interop

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
)

func TestLiveRgostatusAgainstPythonRNSD(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath("rnsd"); err != nil {
		t.Skip("rnsd not installed")
	}
	if _, err := exec.LookPath("rnstatus"); err != nil {
		t.Skip("rnstatus not installed")
	}

	cfgDir := t.TempDir()
	port := freeUDPPort(t)
	ctrlPort := port + 1
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = yes",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
		"[logging]",
		"loglevel = 3",
		"",
		"[interfaces]",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rnsd", "--config", cfgDir)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		t.Fatalf("start rnsd: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		cfg, err := rnsutil.LoadConfigDir(cfgDir)
		if err != nil {
			lastErr = err
			time.Sleep(200 * time.Millisecond)
			continue
		}
		client, err := rnsutil.DialRPC(cfg, nil)
		if err != nil {
			lastErr = err
			time.Sleep(200 * time.Millisecond)
			continue
		}
		client.SetTimeout(2 * time.Second)
		stats, err := client.GetInterfaceStats()
		if err != nil {
			lastErr = err
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if len(stats.TransportID) == 0 {
			lastErr = errEmptyTransportID
			time.Sleep(200 * time.Millisecond)
			continue
		}
		var buf bytes.Buffer
		if err := rnsutil.WriteStatusJSON(&buf, stats); err != nil {
			t.Fatal(err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"interfaces", "rxb", "txb", "transport_id", "rxqt", "rxqd"} {
			if _, ok := parsed[key]; !ok {
				t.Fatalf("missing json key %s in %s", key, buf.String())
			}
		}
		ifaces, _ := parsed["interfaces"].([]any)
		if len(ifaces) == 0 {
			t.Fatal("expected at least one interface from python rnsd")
		}
		first, _ := ifaces[0].(map[string]any)
		for _, key := range []string{
			"incoming_announce_frequency",
			"outgoing_announce_frequency",
			"incoming_pr_frequency",
			"outgoing_pr_frequency",
			"held_announces",
			"protocol_violations",
			"ifac_violations",
			"packet_filter_hits",
		} {
			if _, ok := first[key]; !ok {
				t.Fatalf("missing interface field %s", key)
			}
		}
		return
	}
	t.Fatalf("rgostatus against python rnsd failed: %v", lastErr)
}

var errEmptyTransportID = errString("empty transport id")

type errString string

func (e errString) Error() string { return string(e) }

func TestLiveRgostatusAgainstGoDaemon(t *testing.T) {
	liveOrSkip(t)

	cfgDir := t.TempDir()
	port := freeUDPPort(t)
	ctrlPort := port + 1
	cfgPath := filepath.Join(cfgDir, "config")
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = yes",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	goCfg := &common.ReticulumConfig{
		EnableTransport:     true,
		ShareInstance:       true,
		SharedInstancePort:  port,
		InstanceControlPort: ctrlPort,
		SharedInstanceType:  common.SharedInstanceTCP,
		ConfigPath:          cfgPath,
	}
	n, err := node.New(goCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	cfg, err := rnsutil.LoadConfigDir(cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	client, err := rnsutil.DialRPC(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := client.GetInterfaceStats()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.TransportID) == 0 {
		t.Fatal("empty transport id from go daemon")
	}
	var buf bytes.Buffer
	if err := rnsutil.WriteStatusJSON(&buf, stats); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("incoming_announce_frequency")) {
		t.Fatalf("json missing announce rates: %s", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("rxqt")) {
		t.Fatalf("json missing inbound queue stats: %s", buf.String())
	}
}

func TestLiveRgoidPythonCrossSign(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath("rnid"); err != nil {
		t.Skip("rnid not installed")
	}

	dir := t.TempDir()
	idPath := filepath.Join(dir, "id.rid")
	msgPath := filepath.Join(dir, "msg.txt")
	if err := os.WriteFile(msgPath, []byte("cross sign payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Go signs, Python verifies
	rgoid := filepath.Join(repoRoot(t), "bin", "rgoid")
	if _, err := os.Stat(rgoid); err != nil {
		cmd := exec.Command("go", "build", "-o", rgoid, "./cmd/rgoid")
		cmd.Dir = repoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build rgoid: %v\n%s", err, out)
		}
	}
	if out, err := exec.Command(rgoid, "-g", idPath).CombinedOutput(); err != nil {
		t.Fatalf("rgoid generate: %v\n%s", err, out)
	}
	if out, err := exec.Command(rgoid, "-i", idPath, "-s", msgPath, "-f").CombinedOutput(); err != nil {
		t.Fatalf("rgoid sign: %v\n%s", err, out)
	}
	pyVerify := exec.Command("rnid", "-i", idPath, "-V", msgPath)
	if out, err := pyVerify.CombinedOutput(); err != nil {
		t.Fatalf("python rnid verify go rsg: %v\n%s", err, out)
	}

	// Python signs a second file, Go verifies
	msg2 := filepath.Join(dir, "msg2.txt")
	if err := os.WriteFile(msg2, []byte("python signed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("rnid", "-i", idPath, "-s", msg2, "-f").CombinedOutput(); err != nil {
		t.Fatalf("python rnid sign: %v\n%s", err, out)
	}
	if out, err := exec.Command(rgoid, "-i", idPath, "-V", msg2).CombinedOutput(); err != nil {
		t.Fatalf("rgoid verify python rsg: %v\n%s", err, out)
	}

	// RSM cross
	rsmPath := filepath.Join(dir, "note.rsm")
	if out, err := exec.Command(rgoid, "-i", idPath, "-S", "go rsm body", "-w", rsmPath, "-f").CombinedOutput(); err != nil {
		t.Fatalf("rgoid rsm: %v\n%s", err, out)
	}
	if out, err := exec.Command("rnid", "-V", rsmPath).CombinedOutput(); err != nil {
		t.Fatalf("python verify go rsm: %v\n%s", err, out)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// tests/interop -> repo root
	return filepath.Clean(filepath.Join(wd, "../.."))
}
