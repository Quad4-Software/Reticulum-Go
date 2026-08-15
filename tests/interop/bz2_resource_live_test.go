// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live bz2 resource bomb checks over random directory.rns.recipes TCP peers.
// Set RUN_LIVE_INTEROP=1. Optional: INTEROP_DIRECTORY_URL.

package interop

import (
	"bytes"
	"crypto/sha256"
	"math/rand"
	"runtime"
	"strings"
	"testing"
	"time"

	"quad4/bzip2/pkg/bzip2"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	rlink "quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/resource"
	"quad4/reticulum-go/pkg/transport"
)

func shufflePeers(peers []directoryPeer) []directoryPeer {
	out := append([]directoryPeer(nil), peers...)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func bz2BombLive(t *testing.T, decompressedLen int) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := bzip2.NewWriter(&buf, 9)
	if err != nil {
		t.Fatalf("bzip2.NewWriter: %v", err)
	}
	zeros := make([]byte, 64*1024)
	remaining := decompressedLen
	for remaining > 0 {
		n := min(len(zeros), remaining)
		if _, err := w.Write(zeros[:n]); err != nil {
			t.Fatalf("bzip2 write: %v", err)
		}
		remaining -= n
	}
	if err := w.Close(); err != nil {
		t.Fatalf("bzip2 close: %v", err)
	}
	return buf.Bytes()
}

func rssBytes() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// TestLiveDirectoryBz2ResourceBombRejection connects through random community
// TCP hubs from directory.rns.recipes, then verifies a crafted resource bz2 bomb
// is rejected by assembleIncomingPayload without ballooning heap.
//
// The bomb is exercised against this process only. Community hubs are transport
// uplinks, not attack targets.
func TestLiveDirectoryBz2ResourceBombRejection(t *testing.T) {
	liveOrSkip(t)

	peers := shufflePeers(fetchOnlineClearnetTCPPeers(t, 24))
	t.Logf("directory returned %d clearnet TCP peers (shuffled)", len(peers))

	cfg := &common.ReticulumConfig{EnableTransport: true, ShareInstance: false}
	tr := transport.NewTransport(cfg)
	defer func() { _ = tr.Close() }()
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}

	var connected []directoryPeer
	for _, p := range peers {
		if len(connected) >= 3 {
			break
		}
		iface, err := startDirectoryTCPClient(t, tr, p, true)
		if err != nil {
			t.Logf("skip peer %s (%s:%d): %v", p.Name, p.Host, p.Port, err)
			continue
		}
		_ = iface
		connected = append(connected, p)
		t.Logf("connected to community peer %s (%s:%d)", p.Name, p.Host, p.Port)
	}
	if len(connected) == 0 {
		t.Fatal("could not connect to any random directory TCP peer")
	}

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(id, destination.In|destination.Out, destination.Single, "bz2_bomb_live", tr, "probe")
	if err != nil {
		t.Fatal(err)
	}
	if err := dest.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)

	const claimed = 1024
	const huge = 64 * 1024 * 1024
	bomb := bz2BombLive(t, huge)
	if len(bomb) > 8192 {
		t.Fatalf("bomb compressed size unexpectedly large: %d", len(bomb))
	}
	t.Logf("bomb %d bytes compressed claims expand to %d (ratio %dx) via peers %v",
		len(bomb), huge, huge/len(bomb), connected)

	randomHash := bytes.Repeat([]byte{0xA5}, resource.RandomHashSize)
	inner := append(append([]byte(nil), randomHash...), bomb...)
	adv := &resource.ResourceAdvertisement{
		Compressed: true,
		Encrypted:  false,
		DataSize:   claimed,
		RandomHash: randomHash,
		Hash:       bytes.Repeat([]byte{0x00}, sha256.Size),
	}

	before := rssBytes()
	out, err := rlink.AssembleIncomingResourcePayload(nil, inner, adv)
	if err == nil {
		t.Fatalf("expected bz2 bomb rejection over live mesh context, got %d bytes", len(out))
	}
	if !strings.Contains(err.Error(), "exceeds advertised data_size") {
		t.Fatalf("unexpected rejection error: %v", err)
	}
	after := rssBytes()
	growth := int64(after) - int64(before)
	t.Logf("heap alloc before=%d after=%d growth=%d err=%v", before, after, growth, err)
	if growth > 32*1024*1024 {
		t.Fatalf("heap grew by %d bytes processing bomb (want << 32MiB)", growth)
	}
}
