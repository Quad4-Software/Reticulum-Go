// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
)

// Sharding tests for the RNS 1.6.0 path table sharding semantics: usage
// marking on outbound traffic, never-used linger, slow-interface linger
// multiplier, interface-gone removal, and the unused-path pool cull.

func newShardingTransport(t *testing.T) *Transport {
	t.Helper()
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	t.Cleanup(func() { _ = tr.Close() })
	tr.SetIdentity(mustIdentity(t))
	return tr
}

func fastIface(name string) *relayIface {
	f := newRelayIface(name)
	f.Bitrate = 100_000 // above PathSlowBitrate: not slow
	return f
}

func pathFor(t *testing.T, tr *Transport, dest []byte) *common.Path {
	t.Helper()
	tr.mutex.RLock()
	defer tr.mutex.RUnlock()
	return tr.paths[pathMapKey(dest)]
}

func dataPacket(t *testing.T, dest []byte) *packet.Packet {
	t.Helper()
	pkt := packet.NewPacket(
		packet.DestinationSingle, []byte("sharding-probe"), packet.PacketTypeData,
		packet.ContextNone, packet.PropagationBroadcast,
		packet.HeaderType1, nil, false, packet.FlagUnset,
	)
	pkt.DestinationHash = dest
	pkt.DestinationType = packet.DestinationSingle
	if err := pkt.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	return pkt
}

// TestPathUsedMarkedOnOutbound verifies a data packet routed over a path
// marks it used, and an announce packet does not (Python marks only
// non-announce traffic in Transport.outbound).
func TestPathUsedMarkedOnOutbound(t *testing.T) {
	tr := newShardingTransport(t)
	iface := fastIface("used_out")
	if err := tr.RegisterInterface("used_out", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	dest := bytes.Repeat([]byte{0x71}, 16)
	tr.UpdatePath(dest, bytes.Repeat([]byte{0x72}, 16), "used_out", 1)

	p := pathFor(t, tr, dest)
	if p.EverUsed() {
		t.Fatal("fresh path already marked used")
	}
	if err := tr.SendPacket(dataPacket(t, dest)); err != nil {
		t.Fatalf("SendPacket: %v", err)
	}
	p = pathFor(t, tr, dest)
	if !p.EverUsed() {
		t.Fatal("outbound data packet did not mark path used")
	}
	if got := p.UsedAt(); got.IsZero() || time.Since(got) > time.Minute {
		t.Fatalf("UsedAt = %v, want a recent timestamp", got)
	}

	// An announce packet must not touch the marker.
	dest2 := bytes.Repeat([]byte{0x73}, 16)
	tr.UpdatePath(dest2, bytes.Repeat([]byte{0x74}, 16), "used_out", 1)
	announce := packet.NewPacket(
		packet.DestinationSingle, []byte("announce-probe"), packet.PacketTypeAnnounce,
		packet.ContextNone, packet.PropagationBroadcast,
		packet.HeaderType1, nil, false, packet.FlagUnset,
	)
	announce.DestinationHash = dest2
	announce.DestinationType = packet.DestinationSingle
	if err := announce.Pack(); err != nil {
		t.Fatalf("pack announce: %v", err)
	}
	if err := tr.SendPacket(announce); err != nil {
		t.Fatalf("SendPacket announce: %v", err)
	}
	if p := pathFor(t, tr, dest2); p.EverUsed() {
		t.Fatal("announce packet marked path used")
	}
}

// TestNeverUsedPathLingerCull verifies never-used paths are culled after
// the linger time while used paths survive it.
func TestNeverUsedPathLingerCull(t *testing.T) {
	oldLinger := UnusedPathLinger
	UnusedPathLinger = 2 * time.Second
	t.Cleanup(func() { UnusedPathLinger = oldLinger })

	tr := newShardingTransport(t)
	iface := fastIface("linger_out")
	if err := tr.RegisterInterface("linger_out", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	fresh := bytes.Repeat([]byte{0x81}, 16)
	tr.UpdatePath(fresh, bytes.Repeat([]byte{0x82}, 16), "linger_out", 1)
	stale := bytes.Repeat([]byte{0x83}, 16)
	tr.UpdatePath(stale, bytes.Repeat([]byte{0x84}, 16), "linger_out", 1)
	backdatePath(tr, stale, 5*time.Second)

	// Use the fresh path so its never-used status clears.
	if err := tr.SendPacket(dataPacket(t, fresh)); err != nil {
		t.Fatalf("SendPacket: %v", err)
	}

	tr.cleanupExpiredPaths()

	if pathFor(t, tr, fresh) == nil {
		t.Fatal("used path was culled despite being marked used")
	}
	if pathFor(t, tr, stale) != nil {
		t.Fatal("never-used path survived the linger window")
	}
}

// TestSlowInterfaceLingerMultiplier verifies never-used paths on slow
// interfaces linger PATH_SLOW_LINGER_FACTOR times longer.
func TestSlowInterfaceLingerMultiplier(t *testing.T) {
	oldLinger := UnusedPathLinger
	UnusedPathLinger = 2 * time.Second
	t.Cleanup(func() { UnusedPathLinger = oldLinger })

	tr := newShardingTransport(t)
	fast := fastIface("linger_fast")
	if err := tr.RegisterInterface("linger_fast", fast); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}
	slow := newRelayIface("linger_slow") // Bitrate 5 bps: slow
	if err := tr.RegisterInterface("linger_slow", slow); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	fastDest := bytes.Repeat([]byte{0x91}, 16)
	tr.UpdatePath(fastDest, bytes.Repeat([]byte{0x92}, 16), "linger_fast", 1)
	slowDest := bytes.Repeat([]byte{0x93}, 16)
	tr.UpdatePath(slowDest, bytes.Repeat([]byte{0x94}, 16), "linger_slow", 1)
	backdatePath(tr, fastDest, 5*time.Second)
	backdatePath(tr, slowDest, 5*time.Second) // within 2s*10 for slow ifaces

	tr.cleanupExpiredPaths()

	if pathFor(t, tr, fastDest) != nil {
		t.Fatal("fast-interface never-used path survived past the linger window")
	}
	if pathFor(t, tr, slowDest) == nil {
		t.Fatal("slow-interface never-used path did not get the linger multiplier")
	}

	// Past the multiplied linger the slow path goes too.
	backdatePath(tr, slowDest, 25*time.Second)
	tr.cleanupExpiredPaths()
	if pathFor(t, tr, slowDest) != nil {
		t.Fatal("slow-interface never-used path survived the multiplied linger")
	}
}

// TestPathRemovedWithInterface verifies paths whose attached interface
// disappeared are removed at the next cull (RNS 1.6.0).
func TestPathRemovedWithInterface(t *testing.T) {
	tr := newShardingTransport(t)
	iface := fastIface("gone_out")
	if err := tr.RegisterInterface("gone_out", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	dest := bytes.Repeat([]byte{0xA1}, 16)
	tr.UpdatePath(dest, bytes.Repeat([]byte{0xA2}, 16), "gone_out", 1)

	// Use the path so the never-used linger does not apply.
	if err := tr.SendPacket(dataPacket(t, dest)); err != nil {
		t.Fatalf("SendPacket: %v", err)
	}

	// Simulate the interface vanishing without path bookkeeping.
	if err := iface.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	tr.mutex.Lock()
	delete(tr.interfaces, "gone_out")
	tr.mutex.Unlock()

	tr.cleanupExpiredPaths()
	if pathFor(t, tr, dest) != nil {
		t.Fatal("path survived removal of its attached interface")
	}
}

// TestUnusedPathPoolCull verifies the never-used pool is culled down to
// PATH_POOL_UNUSED_TARGET, non-slow interfaces first and oldest expiry
// first (Python unused_paths.sort() order).
func TestUnusedPathPoolCull(t *testing.T) {
	oldMax := PathPoolUnusedMax
	oldMin := PathPoolUnusedMin
	PathPoolUnusedMax = 3
	PathPoolUnusedMin = 0
	t.Cleanup(func() {
		PathPoolUnusedMax = oldMax
		PathPoolUnusedMin = oldMin
	})

	tr := newShardingTransport(t)
	fast := fastIface("pool_fast")
	if err := tr.RegisterInterface("pool_fast", fast); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}
	slow := newRelayIface("pool_slow")
	if err := tr.RegisterInterface("pool_slow", slow); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	// Five never-used paths: 3 fast with staggered expiry, 1 fast with the
	// earliest expiry, 1 slow. Expect the earliest-expiry fast path and
	// one later fast path culled; the slow path retained.
	type destInfo struct {
		dest   []byte
		name   string
		expiry time.Duration
	}
	infos := []destInfo{
		{bytes.Repeat([]byte{0xB1}, 16), "pool_fast", 7 * 24 * time.Hour},
		{bytes.Repeat([]byte{0xB2}, 16), "pool_fast", 1 * time.Hour},
		{bytes.Repeat([]byte{0xB3}, 16), "pool_fast", 3 * time.Hour},
		{bytes.Repeat([]byte{0xB4}, 16), "pool_fast", 6 * 24 * time.Hour},
		{bytes.Repeat([]byte{0xB5}, 16), "pool_slow", 2 * time.Hour},
	}
	for _, info := range infos {
		tr.UpdatePath(info.dest, bytes.Repeat([]byte{0xB6}, 16), info.name, 1)
		tr.mutex.Lock()
		p := tr.paths[pathMapKey(info.dest)]
		p.Expires = time.Now().Add(info.expiry)
		tr.mutex.Unlock()
	}
	// Force the pool check to run on this cull.
	tr.mutex.Lock()
	tr.pathPoolLastChecked = time.Time{}
	tr.mutex.Unlock()

	tr.cleanupExpiredPaths()

	survivors := map[string]bool{}
	for _, info := range infos {
		if pathFor(t, tr, info.dest) != nil {
			survivors[fmtHash16(info.dest)] = true
		}
	}
	if got := len(survivors); got != PathPoolUnusedMax {
		t.Fatalf("survivors = %d, want %d", got, PathPoolUnusedMax)
	}
	if survivors[fmtHash16(infos[1].dest)] {
		t.Fatal("earliest-expiry fast path was not culled first")
	}
	if survivors[fmtHash16(infos[2].dest)] {
		t.Fatal("second-earliest fast path was not culled")
	}
	if !survivors[fmtHash16(infos[0].dest)] {
		t.Fatal("later fast path was culled out of order")
	}
	if !survivors[fmtHash16(infos[3].dest)] {
		t.Fatal("later fast path was culled out of order")
	}
	if !survivors[fmtHash16(infos[4].dest)] {
		t.Fatal("slow-interface path was culled before fast ones")
	}
}

func fmtHash16(h []byte) string {
	out := make([]byte, 0, 32)
	const hexDigits = "0123456789abcdef"
	for _, b := range h {
		out = append(out, hexDigits[b>>4], hexDigits[b&0x0f])
	}
	return string(out)
}
