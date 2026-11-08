// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/stamps"
)

func TestParsePathRequestWireSizes(t *testing.T) {
	mk := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(i)
		}
		return b
	}
	// Stamped 64-byte form.
	d, tid, tag, ext, ok := parsePathRequestWire(mk(64))
	if !ok || len(d) != 16 || len(tid) != 16 || len(tag) != 16 || len(ext) != 16 {
		t.Fatal("64-byte stamped form misparse")
	}
	// 48-byte modern form, no ext.
	d, tid, tag, ext, ok = parsePathRequestWire(mk(48))
	if !ok || len(d) != 16 || len(tid) != 16 || len(tag) != 16 || ext != nil {
		t.Fatal("48-byte form misparse")
	}
	// 33..63: tid + variable tag truncated to 16.
	d, tid, tag, ext, ok = parsePathRequestWire(mk(40))
	if !ok || len(tid) != 16 || len(tag) != 8 || ext != nil {
		t.Fatalf("40-byte form: tid=%d tag=%d ext=%v", len(tid), len(tag), ext)
	}
	d, tid, tag, ext, ok = parsePathRequestWire(mk(50))
	if !ok || len(tag) != 16 {
		t.Fatalf("50-byte form tag=%d want 16", len(tag))
	}
	// Legacy 32-byte dst|tag (no tid).
	d, tid, tag, ext, ok = parsePathRequestWire(mk(32))
	if !ok || tid != nil || len(tag) != 16 {
		t.Fatal("32-byte legacy misparse")
	}
	// 17..31 legacy variable tag.
	d, tid, tag, ext, ok = parsePathRequestWire(mk(20))
	if !ok || tid != nil || len(tag) != 4 {
		t.Fatalf("20-byte legacy tag=%d", len(tag))
	}
	// Tagless fails.
	if _, _, _, _, ok = parsePathRequestWire(mk(16)); ok {
		t.Fatal("tagless must fail")
	}
	if _, _, _, _, ok = parsePathRequestWire(mk(8)); ok {
		t.Fatal("too short must fail")
	}
}

func TestPRStampValidationAndEnforcedDrop(t *testing.T) {
	destHash := bytes.Repeat([]byte{0x11}, 16)
	tag := bytes.Repeat([]byte{0x22}, 16)
	material := append(append([]byte{}, destHash...), tag...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stamp, _, err := stamps.GenerateStamp(ctx, material, 8, stamps.StampLength)
	if err != nil {
		t.Fatal(err)
	}
	if !validatePRStamp(stamp, destHash, tag, 8) {
		t.Fatal("stamp should validate at cost 8")
	}
	if validatePRStamp(stamp, destHash, tag, 99) {
		t.Fatal("stamp must not validate at cost 99")
	}
	if validatePRStamp(stamp[:10], destHash, tag, 8) {
		t.Fatal("short stamp must fail")
	}
	if validatePRStamp(stamp, destHash, bytes.Repeat([]byte{0x33}, 16), 8) {
		t.Fatal("wrong material must fail")
	}

	// Enforced drop: stamped PR at cost 6 vs required 23.5 default.
	tr, iface, _ := newStampTestTransport(t, common.StampsConfig{EnforceStamps: true})
	data := append(append(append([]byte{}, destHash...), tr.prTransportID()...), tag...)
	// Unstamped 48-byte PR dropped under enforcement.
	tr.handlePathRequest(data, iface)
	// Valid-stamped 64-byte PR at low cost also dropped (below pr_cost 15.8 default).
	loStamp, _, err := stamps.GenerateStamp(ctx, material, 6, stamps.StampLength)
	if err != nil {
		t.Fatal(err)
	}
	data2 := append(append(append([]byte{}, data...), stamps.ExtPayloadMarker, 0x00), loStamp...)
	tr.handlePathRequest(data2, iface)
	// Oversize is a violation and does not panic.
	tr.handlePathRequest(make([]byte, 65), iface)
}

func TestPRStampCache(t *testing.T) {
	c := &prStampCache{}
	for i := 0; i < prStampCacheMax+10; i++ {
		m := []byte{byte(i), byte(i >> 8)}
		c.put(m, []byte{0xAA})
	}
	c.Lock()
	n := len(c.m)
	c.Unlock()
	if n != prStampCacheMax {
		t.Fatalf("cache size %d want %d", n, prStampCacheMax)
	}
	if _, ok := c.get([]byte{0, 0}); ok {
		t.Fatal("oldest entry should have been evicted")
	}
}

func TestStampedPREmission(t *testing.T) {
	cfg := &common.ReticulumConfig{EnableTransport: true, Stamps: common.StampsConfig{
		StampPRs: true, PRValue: 6, PRValueSet: true, PRStampTimeout: 30, PRStampTimeoutSet: true,
	}}
	tr := NewTransport(cfg)
	tr.SetIdentity(mustStampIdentity(t))
	out := newRelayIface("prout")
	out.Online = true
	if err := tr.RegisterInterface(out.GetName(), out); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })

	destHash := bytes.Repeat([]byte{0x44}, 16)
	if err := tr.RequestPath(destHash, out.GetName(), nil, false); err != nil {
		t.Fatalf("RequestPath: %v", err)
	}
	// Emission is deferred until the stamp is ready.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		out.mu.Lock()
		n := len(out.sent)
		out.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	out.mu.Lock()
	pkts := append([][]byte(nil), out.sent...)
	out.mu.Unlock()
	if len(pkts) == 0 {
		t.Fatal("stamped PR never emitted")
	}
	pkt := pkts[0]
	// Extract DATA payload from the wire packet: header(2)+dest(16)+ctx(1)
	data := pkt[19:]
	if len(data) != 64 {
		t.Fatalf("stamped PR data len=%d want 64", len(data))
	}
	d, tid, tag, ext, ok := parsePathRequestWire(data)
	if !ok || ext == nil || ext[0] != stamps.ExtPayloadMarker || ext[1] != 0x00 {
		t.Fatal("missing/malformed ext")
	}
	if !bytes.Equal(d, destHash) || len(tid) != 16 {
		t.Fatal("bad dst/tid")
	}
	if !validatePRStamp(ext[2:], d, tag, 6) {
		t.Fatal("emitted stamp does not validate")
	}
}
