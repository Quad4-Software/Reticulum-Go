// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"testing"
	"time"

	"git.quad4.io/libraries/msgpack/v5/pkg/msgpack"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
)

func newStatsTestRouter(t *testing.T) (*Router, *transport.Transport, *identity.Identity) {
	t.Helper()
	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Propagation.EnableNode = true
	router, err := NewRouter(id, tr, RouterOptions{
		Config:      cfg,
		StoragePath: t.TempDir(),
		MessagesDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		router.Stop()
		_ = tr.Close()
	})
	return router, tr, id
}

func addStatsTestPeer(r *Router, hash []byte, static bool) {
	if static {
		r.staticPeers[peerKey(hash)] = struct{}{}
	}
	r.peer(hash, time.Now().Unix(), 256, 512, 16, 3, 18, nil)
}

func TestPropagationStatsWirePeerKeysAreBinary(t *testing.T) {
	router, _, id := newStatsTestRouter(t)
	if err := router.EnablePropagation(); err != nil {
		t.Fatal(err)
	}

	staticHash := bytes.Repeat([]byte{0x31}, DestinationLength)
	discoveredHash := bytes.Repeat([]byte{0x42}, DestinationLength)
	addStatsTestPeer(router, staticHash, true)
	addStatsTestPeer(router, discoveredHash, false)

	router.controlAllow[peerKey(id.Hash())] = struct{}{}
	resp := router.statsGetRequestHandler("", nil, nil, nil, id, 0)
	wire, err := msgpack.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal stats response: %v", err)
	}
	for name, hash := range map[string][]byte{"static": staticHash, "discovered": discoveredHash} {
		pattern := append([]byte{0xc4, byte(DestinationLength)}, hash...)
		if !bytes.Contains(wire, pattern) {
			t.Fatalf("%s peer key is not encoded as msgpack bin", name)
		}
		strPattern := append([]byte{0xa0 + byte(DestinationLength)}, hash...)
		if bytes.Contains(wire, strPattern) {
			t.Fatalf("%s peer key encoded as msgpack str", name)
		}
	}

	var decoded any
	if err := msgpack.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("decode stats response: %v", err)
	}
	if decoded == nil {
		t.Fatal("decoded stats response is nil")
	}

	local := router.PropagationStats()
	peers, ok := local["peers"].(map[string]any)
	if !ok {
		t.Fatalf("local peers type %T", local["peers"])
	}
	for name, hash := range map[string][]byte{"static": staticHash, "discovered": discoveredHash} {
		key := peerKey(hash)
		raw, ok := peers[key]
		if !ok {
			t.Fatalf("missing local %s peer key %s", name, key)
		}
		p := raw.(map[string]any)
		if p["type"] != name {
			t.Fatalf("%s peer type %v", name, p["type"])
		}
	}
	if local["total_peers"] != 2 || local["static_peers"] != 1 || local["discovered_peers"] != 1 {
		t.Fatalf("unexpected peer counts: %v %v %v", local["total_peers"], local["static_peers"], local["discovered_peers"])
	}
}

func TestPropagationStatsWireZeroPeers(t *testing.T) {
	router, _, id := newStatsTestRouter(t)
	if err := router.EnablePropagation(); err != nil {
		t.Fatal(err)
	}
	router.controlAllow[peerKey(id.Hash())] = struct{}{}
	resp := router.statsGetRequestHandler("", nil, nil, nil, id, 0)
	wire, err := msgpack.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal zero-peer stats response: %v", err)
	}
	var decoded any
	if err := msgpack.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("decode zero-peer stats response: %v", err)
	}
	local := router.PropagationStats()
	peers, ok := local["peers"].(map[string]any)
	if !ok {
		t.Fatalf("local zero peers type %T", local["peers"])
	}
	if len(peers) != 0 {
		t.Fatalf("expected no peers, got %d", len(peers))
	}
}

func TestRecoverRequestHandlerReturnsNil(t *testing.T) {
	wrapped := recoverRequestHandler("panic-test", func(string, []byte, []byte, []byte, *identity.Identity, int64) any {
		panic("boom")
	})
	if got := wrapped("", nil, nil, nil, nil, 0); got != nil {
		t.Fatalf("panic recovery returned %T", got)
	}
}
