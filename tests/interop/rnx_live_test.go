// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live rnx / rgox interop: Go↔Go and Go↔Python over UDP.
// Set RUN_LIVE_INTEROP=1 to enable. Python tests also need rnx on PATH
// (or RETICULUM_PATH pointing at a Reticulum checkout with RNS.Utilities.rnx).

package interop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	rlink "quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
)

func writeUDPPeerConfig(t *testing.T, dir string, listen, peerPort int) {
	t.Helper()
	cfg := strings.Join([]string{
		"[reticulum]",
		"enable_transport = yes",
		"share_instance = no",
		"",
		"[interfaces]",
		"  [[UDP]]",
		"    type = UDPInterface",
		"    enabled = yes",
		"    listen_ip = 127.0.0.1",
		"    listen_port = " + strconv.Itoa(listen),
		"    target_host = 127.0.0.1",
		"    target_port = " + strconv.Itoa(peerPort),
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

func ensureRgox(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(repoRoot(t), "bin", "rgox")
	if _, err := os.Stat(bin); err != nil {
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/rgox")
		cmd.Dir = repoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build rgox: %v\n%s", err, out)
		}
	}
	return bin
}

func TestLiveGoToGoRNXCommand(t *testing.T) {
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
	idClient, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}

	dest, err := destination.New(idListen, destination.In, destination.Single, rnsutil.RNXAppName, nA.Transport(), rnsutil.RNXAspect)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)
	_ = dest.RegisterRequestHandlerAny(rnsutil.RNXCommandPath, func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
		req, err := rnsutil.ParseRNXRequestPayload(data)
		if err != nil {
			return rnsutil.PackRNXResult(rnsutil.RNXResult{})
		}
		return rnsutil.PackRNXResult(rnsutil.ExecuteRNXCommandLocally(req))
	}, destination.AllowAll, nil)
	_ = dest.Announce(false, nil, nil)

	cfgB, err := rnsutil.LoadConfigDir(cfgDirB)
	if err != nil {
		t.Fatal(err)
	}
	nB, err := node.New(cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if err := nB.Start(); err != nil {
		t.Fatal(err)
	}
	defer nB.Stop()

	destHash := dest.GetHash()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := rnsutil.WaitPath(ctx, nB.Transport(), destHash); err != nil {
		t.Fatalf("path: %v", err)
	}
	l, err := rnsutil.EstablishRNXLink(ctx, nB.Transport(), destHash)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Teardown()
	if err := l.Identify(idClient); err != nil {
		t.Fatal(err)
	}

	to := 10.0
	req := rnsutil.RNXRequest{Command: "echo go-to-go-rnx", TimeoutSec: &to}
	receipt, err := l.Request(rnsutil.RNXCommandPath, rnsutil.PackRNXRequest(req), rnsutil.RNXRequestTimeout(15*time.Second, l.RTT()))
	if err != nil {
		t.Fatal(err)
	}
	if err := rnsutil.WaitRequest(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.GetStatus() == rlink.StatusFailed {
		t.Fatal("request failed")
	}
	res, err := rnsutil.ParseRNXResult(receipt.GetResponseValue())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Executed || !bytes.Contains(res.Stdout, []byte("go-to-go-rnx")) {
		t.Fatalf("result=%+v stdout=%q", res, res.Stdout)
	}
}

func TestLiveRgoxCLIAgainstGoListenerUDP(t *testing.T) {
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
	dest, err := destination.New(idListen, destination.In, destination.Single, rnsutil.RNXAppName, nA.Transport(), rnsutil.RNXAspect)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)
	_ = dest.RegisterRequestHandlerAny(rnsutil.RNXCommandPath, func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
		req, err := rnsutil.ParseRNXRequestPayload(data)
		if err != nil {
			return rnsutil.PackRNXResult(rnsutil.RNXResult{})
		}
		return rnsutil.PackRNXResult(rnsutil.ExecuteRNXCommandLocally(req))
	}, destination.AllowAll, nil)
	_ = dest.Announce(false, nil, nil)

	rgox := ensureRgox(t)
	destHex := hex.EncodeToString(dest.GetHash())
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	// -N skips identify; listener uses AllowAll
	cmd := exec.CommandContext(ctx, rgox, "-config", cfgDirB, "-N", "-json", destHex, "echo", "cli-rnx-ok")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rgox: %v\n%s", err, out)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		// stdout may include path request line before JSON
		idx := bytes.IndexByte(out, '{')
		if idx < 0 {
			t.Fatalf("no json in %q", out)
		}
		if err := json.Unmarshal(out[idx:], &payload); err != nil {
			t.Fatalf("json: %v\n%s", err, out)
		}
	}
	if payload["executed"] != true {
		t.Fatalf("payload=%v", payload)
	}
	if !strings.Contains(fmtString(payload["stdout"]), "cli-rnx-ok") {
		t.Fatalf("stdout=%v", payload["stdout"])
	}
}

func fmtString(v any) string {
	s, _ := v.(string)
	return s
}

func TestLiveGoRNXListenerPythonRNXClient(t *testing.T) {
	liveOrSkip(t)
	script := filepath.Join(repoRoot(t), "tests", "interop", "py", "rnx_client.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("rnx_client.py missing")
	}
	if os.Getenv("RETICULUM_PATH") == "" {
		if _, err := exec.LookPath("rnx"); err != nil {
			t.Skip("need RETICULUM_PATH or rnx on PATH for Python client")
		}
	}

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
	dest, err := destination.New(idListen, destination.In, destination.Single, rnsutil.RNXAppName, nA.Transport(), rnsutil.RNXAspect)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)
	_ = dest.RegisterRequestHandlerAny(rnsutil.RNXCommandPath, func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
		req, err := rnsutil.ParseRNXRequestPayload(data)
		if err != nil {
			return rnsutil.PackRNXResult(rnsutil.RNXResult{})
		}
		return rnsutil.PackRNXResult(rnsutil.ExecuteRNXCommandLocally(req))
	}, destination.AllowAll, nil)
	_ = dest.Announce(false, nil, nil)

	destHex := hex.EncodeToString(dest.GetHash())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonExe(), script)
	cmd.Env = append(os.Environ(),
		"INTEROP_CONFIG_DIR="+cfgDirB,
		"INTEROP_LISTEN_PORT="+strconv.Itoa(portB),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(portA),
		"INTEROP_DEST_HASH="+destHex,
		"INTEROP_COMMAND=echo py-client-ok",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python rnx client: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("py-client-ok")) {
		t.Fatalf("missing output: %s", out)
	}
}

func TestLivePythonRNXListenerGoXClient(t *testing.T) {
	liveOrSkip(t)
	script := filepath.Join(repoRoot(t), "tests", "interop", "py", "rnx_listen.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("rnx_listen.py missing")
	}

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	cfgDirA := t.TempDir()
	cfgDirB := t.TempDir()
	writeUDPPeerConfig(t, cfgDirA, portA, portB)
	writeUDPPeerConfig(t, cfgDirB, portB, portA)

	ctx, cancel := context.WithTimeout(context.Background(), pyProcMediumTimeout)
	defer cancel()
	py := exec.CommandContext(ctx, pythonExe(), script)
	py.Env = append(os.Environ(),
		"INTEROP_CONFIG_DIR="+cfgDirA,
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

	destHex, err := readReadyLine(t, stdout, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	rgox := ensureRgox(t)
	cmd := exec.CommandContext(ctx, rgox, "-config", cfgDirB, "-N", "-json", destHex, "echo", "go-client-ok")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rgox against python: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("go-client-ok")) {
		t.Fatalf("missing output: %s", out)
	}
}

func TestLiveRNXAllowListRejects(t *testing.T) {
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
	allowedID, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	badID, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}

	dest, err := destination.New(idListen, destination.In, destination.Single, rnsutil.RNXAppName, nA.Transport(), rnsutil.RNXAspect)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)
	dest.SetLinkEstablishedCallback(func(lnk any) {
		l := interopLink(t, lnk)
		l.SetRemoteIdentifiedCallback(func(lnk *rlink.Link, remote *identity.Identity) {
			if remote == nil || !rnsutil.AllowedContains([][]byte{allowedID.Hash()}, remote.Hash()) {
				lnk.Teardown()
			}
		})
	})
	_ = dest.RegisterRequestHandlerAny(rnsutil.RNXCommandPath, func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
		req, err := rnsutil.ParseRNXRequestPayload(data)
		if err != nil {
			return rnsutil.PackRNXResult(rnsutil.RNXResult{})
		}
		return rnsutil.PackRNXResult(rnsutil.ExecuteRNXCommandLocally(req))
	}, destination.AllowList, [][]byte{allowedID.Hash()})
	_ = dest.Announce(false, nil, nil)

	cfgB, err := rnsutil.LoadConfigDir(cfgDirB)
	if err != nil {
		t.Fatal(err)
	}
	nB, err := node.New(cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if err := nB.Start(); err != nil {
		t.Fatal(err)
	}
	defer nB.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	l, err := rnsutil.EstablishRNXLink(ctx, nB.Transport(), dest.GetHash())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Teardown()
	_ = l.Identify(badID)
	to := 5.0
	receipt, err := l.Request(rnsutil.RNXCommandPath, rnsutil.PackRNXRequest(rnsutil.RNXRequest{Command: "echo no", TimeoutSec: &to}), 8*time.Second)
	if err == nil {
		_ = rnsutil.WaitRequest(ctx, receipt)
		if receipt.GetStatus() != rlink.StatusFailed && receipt.GetResponseValue() != nil {
			res, _ := rnsutil.ParseRNXResult(receipt.GetResponseValue())
			if res.Executed {
				t.Fatal("unauthorized identity should not execute")
			}
		}
	}
}

func TestLiveRNXLargeOutput(t *testing.T) {
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
	dest, err := destination.New(idListen, destination.In, destination.Single, rnsutil.RNXAppName, nA.Transport(), rnsutil.RNXAspect)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)
	_ = dest.RegisterRequestHandlerAny(rnsutil.RNXCommandPath, func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
		req, err := rnsutil.ParseRNXRequestPayload(data)
		if err != nil {
			return rnsutil.PackRNXResult(rnsutil.RNXResult{})
		}
		return rnsutil.PackRNXResult(rnsutil.ExecuteRNXCommandLocally(req))
	}, destination.AllowAll, nil)
	_ = dest.Announce(false, nil, nil)

	cfgB, err := rnsutil.LoadConfigDir(cfgDirB)
	if err != nil {
		t.Fatal(err)
	}
	nB, err := node.New(cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if err := nB.Start(); err != nil {
		t.Fatal(err)
	}
	defer nB.Stop()

	idClient, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	l, err := rnsutil.EstablishRNXLink(ctx, nB.Transport(), dest.GetHash())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Teardown()
	_ = l.Identify(idClient)

	to := 30.0
	// ~8KiB of output to force resource response path on small MDU links
	req := rnsutil.RNXRequest{Command: "python3 -c 'print(\"X\"*8192)'", TimeoutSec: &to}
	receipt, err := l.Request(rnsutil.RNXCommandPath, rnsutil.PackRNXRequest(req), rnsutil.RNXRequestTimeout(30*time.Second, l.RTT()))
	if err != nil {
		t.Fatal(err)
	}
	if err := rnsutil.WaitRequest(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	res, err := rnsutil.ParseRNXResult(receipt.GetResponseValue())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Executed || res.StdoutTotal < 8000 {
		t.Fatalf("large result=%+v len=%d", res, len(res.Stdout))
	}
}

func readReadyLine(t *testing.T, r io.Reader, timeout time.Duration) (string, error) {
	t.Helper()
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if after, ok := strings.CutPrefix(line, "READY "); ok {
				ch <- result{line: strings.TrimSpace(after)}
				return
			}
			if line == "READY" {
				ch <- result{line: ""}
				return
			}
		}
		ch <- result{err: sc.Err()}
	}()
	select {
	case res := <-ch:
		return res.line, res.err
	case <-time.After(timeout):
		return "", context.DeadlineExceeded
	}
}
