// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// startInteropProcess runs the Python harness asynchronously for long-lived
// commands such as live_prop_node. The returned wait function blocks until
// the process exits and decodes the final JSON result.
func startInteropProcess(t *testing.T, req map[string]any) (wait func() (map[string]any, string, error)) {
	t.Helper()
	requireInterop(t)

	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("uv", "run", "python", "harness.py")
	cmd.Dir = interopDir
	cmd.Stdin = bytes.NewReader(payload)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start harness: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	return func() (map[string]any, string, error) {
		if err := cmd.Wait(); err != nil {
			return nil, out.String(), fmt.Errorf("harness exit: %w: %s", err, out.String())
		}
		raw := bytes.TrimSpace(out.Bytes())
		if i := bytes.LastIndexByte(raw, '{'); i > 0 {
			raw = raw[i:]
		}
		var resp map[string]any
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, out.String(), fmt.Errorf("decode harness output: %w: %s", err, raw)
		}
		return resp, out.String(), nil
	}
}

func waitReadyFile(t *testing.T, path string, keys ...string) map[string]string {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		raw, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- test ready file
		if err == nil && len(raw) > 0 {
			var out map[string]string
			if json.Unmarshal(raw, &out) == nil {
				complete := true
				for _, k := range keys {
					if out[k] == "" {
						complete = false
						break
					}
				}
				if complete {
					return out
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for ready file %s", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestInterop_Live_GoFetchPythonNode(t *testing.T) {
	if testing.Short() {
		t.Skip("live propagation fetch interop skipped in -short mode")
	}
	requireInterop(t)

	const (
		goPort = 43320
		pyPort = 43321
	)

	tmp := t.TempDir()
	readyPath := filepath.Join(tmp, "ready.json")
	stopPath := filepath.Join(tmp, "stop")

	cfg := common.DefaultConfig()
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	iface := startLXMFUDP(t, "Go-fetch", "127.0.0.1:43320", "127.0.0.1:43321", tr)
	defer iface.Stop()

	id := mustNewIdentity(t)
	dest, err := NewDeliveryDestination(id, tr)
	if err != nil {
		t.Fatal(err)
	}
	m := NewMessenger(tr, dest)

	wantText := "stored on python node"
	wait := startInteropProcess(t, map[string]any{
		"cmd":                  "live_prop_node",
		"listen_port":          pyPort,
		"forward_port":         goPort,
		"ready_path":           readyPath,
		"stop_path":            stopPath,
		"recipient_hash":       hex.EncodeToString(dest.GetHash()),
		"recipient_public_key": hex.EncodeToString(id.GetPublicKey()),
		"text":                 wantText,
		"duration_s":           120,
		"loglevel":             6,
	})

	var ready map[string]string
	deadline := time.Now().Add(30 * time.Second)
	for {
		raw, err := os.ReadFile(filepath.Clean(readyPath)) // #nosec G304 -- test ready file
		if err == nil && len(raw) > 0 {
			var out map[string]string
			if json.Unmarshal(raw, &out) == nil && out["propagation_hash"] != "" {
				ready = out
				break
			}
		}
		if time.Now().After(deadline) {
			_ = os.WriteFile(stopPath, []byte("done"), 0o600)
			_, rawOut, _ := wait()
			t.Fatalf("timeout waiting for python node ready\n%s", rawOut)
		}
		time.Sleep(50 * time.Millisecond)
	}
	pnHash, err := hex.DecodeString(ready["propagation_hash"])
	if err != nil || len(pnHash) != DestinationLength {
		t.Fatalf("propagation_hash=%q err=%v", ready["propagation_hash"], err)
	}

	// The Python node announces its propagation destination; wait for the
	// path and identity to land.
	deadline = time.Now().Add(30 * time.Second)
	for !tr.HasPath(pnHash) {
		if time.Now().After(deadline) {
			t.Fatal("no path to python propagation node")
		}
		_ = dest.Announce(false, nil, nil)
		time.Sleep(100 * time.Millisecond)
	}
	deadline = time.Now().Add(30 * time.Second)
	for {
		if rid, rerr := identity.Recall(pnHash); rerr == nil && rid != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("propagation node identity not announced")
		}
		time.Sleep(100 * time.Millisecond)
	}

	received := make(chan *LXMessage, 4)
	m.SetMessageHandler(func(msg *LXMessage, _ common.NetworkInterface) {
		received <- msg
	})

	n, err := m.FetchPropagated(pnHash, FetchAllMessages, 0)
	if err != nil {
		_ = os.WriteFile(stopPath, []byte("done"), 0o600)
		_, raw, _ := wait()
		t.Fatalf("FetchPropagated: %v\n%s", err, raw)
	}
	if n != 1 {
		_ = os.WriteFile(stopPath, []byte("done"), 0o600)
		_, raw, _ := wait()
		t.Fatalf("expected 1 fetched message, got %d\n%s", n, raw)
	}

	select {
	case got := <-received:
		if got.ContentString() != wantText {
			t.Fatalf("unexpected content %q", got.ContentString())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fetched message was not delivered to handler")
	}

	// The haves acknowledgement purges the entry on the Python node.
	time.Sleep(2 * time.Second)
	if err := os.WriteFile(stopPath, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp, raw, err := wait()
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("python node failed: %v\n%s", resp["error"], raw)
	}
	if served, _ := resp["served"].(float64); served < 1 {
		t.Fatalf("python node did not serve the message: %+v\n%s", resp, raw)
	}
	if purged, _ := resp["purged"].(bool); !purged {
		t.Fatalf("python node did not purge fetched entry: %+v\n%s", resp, raw)
	}
}

func TestInterop_Live_PythonFetchGoNode(t *testing.T) {
	if testing.Short() {
		t.Skip("live propagation fetch interop skipped in -short mode")
	}
	requireInterop(t)

	const (
		goPort = 43322
		pyPort = 43323
	)

	tmp := t.TempDir()
	readyPath := filepath.Join(tmp, "ready.json")

	cfg := common.DefaultConfig()
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	iface := startLXMFUDP(t, "Go-pn", "127.0.0.1:43322", "127.0.0.1:43323", tr)
	defer iface.Stop()

	routerID := mustNewIdentity(t)
	routerCfg := DefaultConfig()
	router, err := NewRouter(routerID, tr, RouterOptions{
		Config:      routerCfg,
		StoragePath: filepath.Join(tmp, "storage"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.EnablePropagation(); err != nil {
		t.Fatal(err)
	}
	router.Start()
	pnDest := router.PropagationDestination()
	pnHash := pnDest.GetHash()

	wait := startInteropProcess(t, map[string]any{
		"cmd":              "live_prop_fetch",
		"listen_port":      pyPort,
		"forward_port":     goPort,
		"ready_path":       readyPath,
		"propagation_hash": hex.EncodeToString(pnHash),
		"timeout_s":        90,
	})

	ready := waitReadyFile(t, readyPath, "delivery_hash", "public_key")
	pyDestHash, err := hex.DecodeString(ready["delivery_hash"])
	if err != nil || len(pyDestHash) != DestinationLength {
		t.Fatalf("delivery_hash=%q err=%v", ready["delivery_hash"], err)
	}
	pyPubKey, err := hex.DecodeString(ready["public_key"])
	if err != nil || len(pyPubKey) == 0 {
		t.Fatalf("public_key=%q err=%v", ready["public_key"], err)
	}

	// Store a message addressed to the Python client's delivery hash.
	srcID := mustNewIdentity(t)
	srcDest, err := NewDeliveryDestination(srcID, tr)
	if err != nil {
		t.Fatal(err)
	}
	wantText := "fetched by python client"
	msg, err := NewMessage(pyDestHash, srcDest.GetHash(), []byte("go-pn"), []byte(wantText), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := msg.Pack(srcID); err != nil {
		t.Fatal(err)
	}
	rid := identity.FromPublicKey(pyPubKey)
	outDest, err := destination.FromHash(pyDestHash, rid, destination.Single, tr)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := outDest.Encrypt(msg.Packed[DestinationLength:])
	if err != nil {
		t.Fatal(err)
	}
	lxmfData := append(append([]byte(nil), pyDestHash...), enc...)
	if _, err := router.store.Add(lxmfData, make([]byte, StampSize), 16); err != nil {
		t.Fatal(err)
	}

	// Keep announcing the node until the Python fetch completes.
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				_ = pnDest.Announce(false, nil, nil)
			}
		}
	}()
	defer close(done)

	resp, raw, err := wait()
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("python fetch failed: %v\n%s", resp["error"], raw)
	}
	if state, _ := resp["state"].(float64); int(state) != 0x07 {
		t.Fatalf("python transfer state=%v, want PR_COMPLETE(7): %+v", state, resp)
	}
	texts, _ := resp["texts"].([]any)
	found := false
	for _, tv := range texts {
		if s, _ := tv.(string); s == wantText {
			found = true
		}
	}
	if !found {
		t.Fatalf("python client did not receive stored message: %+v", resp)
	}

	// The client returns received ids; the Go node must purge the entry.
	deadline := time.Now().Add(10 * time.Second)
	for router.store.Count() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("go node did not purge fetched message")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
