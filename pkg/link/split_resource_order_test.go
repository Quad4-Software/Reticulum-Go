// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

func splitTestLink() *Link {
	tr := transport.NewTransport(&common.ReticulumConfig{
		InMemoryStorage:          true,
		MaxInMemoryResourceBytes: 1 << 20,
		ShareInstance:            false,
	})
	return &Link{
		transport: tr,
		linkID:    []byte{0x42, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
	}
}

func splitAdv(hash []byte, idx, total uint16, dataSize int64) *resource.ResourceAdvertisement {
	return &resource.ResourceAdvertisement{
		OriginalHash:  hash,
		SegmentIndex:  idx,
		TotalSegments: total,
		DataSize:      dataSize,
	}
}

func TestSplitResourceRejectsLateStart(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	l := splitTestLink()
	hash := make([]byte, 32)
	hash[0] = 0x01

	// A transfer that begins at segment 2 can only assemble garbage.
	if err := l.handleSplitSegmentComplete([]byte("data"), splitAdv(hash, 2, 3, 0), nil); err == nil {
		t.Fatal("segment 2 start was not rejected")
	}
}

func TestSplitResourceRejectsOutOfOrder(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	l := splitTestLink()
	hash := make([]byte, 32)
	hash[0] = 0x02

	if err := l.handleSplitSegmentComplete([]byte("A"), splitAdv(hash, 1, 3, 0), nil); err != nil {
		t.Fatal(err)
	}
	if err := l.handleSplitSegmentComplete([]byte("C"), splitAdv(hash, 3, 3, 0), nil); err == nil {
		t.Fatal("skipped segment index was not rejected")
	}
}

func TestSplitResourceRejectsReplay(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	var got []byte
	l := splitTestLink()
	l.resourceConcludedCallback = func(v any) {
		if b, ok := v.([]byte); ok {
			got = append([]byte(nil), b...)
		}
	}
	hash := make([]byte, 32)
	hash[0] = 0x03

	if err := l.handleSplitSegmentComplete([]byte("AA"), splitAdv(hash, 1, 2, 0), nil); err != nil {
		t.Fatal(err)
	}
	if err := l.handleSplitSegmentComplete([]byte("BB"), splitAdv(hash, 2, 2, 0), nil); err != nil {
		t.Fatal(err)
	}
	if string(got) != "AABB" {
		t.Fatalf("assembled %q", got)
	}
	// A replayed final segment after completion must not append again.
	if err := l.handleSplitSegmentComplete([]byte("BB"), splitAdv(hash, 2, 2, 0), nil); err == nil {
		t.Fatal("replayed segment was not rejected")
	}
}

func TestSplitResourceRestartAtSegmentOne(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	var got []byte
	l := splitTestLink()
	l.resourceConcludedCallback = func(v any) {
		if b, ok := v.([]byte); ok {
			got = append([]byte(nil), b...)
		}
	}
	hash := make([]byte, 32)
	hash[0] = 0x04

	if err := l.handleSplitSegmentComplete([]byte("STALE"), splitAdv(hash, 1, 2, 0), nil); err != nil {
		t.Fatal(err)
	}
	// Sender restarts: segment 1 again must reset staging, not append.
	if err := l.handleSplitSegmentComplete([]byte("AA"), splitAdv(hash, 1, 2, 0), nil); err != nil {
		t.Fatalf("restart rejected: %v", err)
	}
	if err := l.handleSplitSegmentComplete([]byte("BB"), splitAdv(hash, 2, 2, 0), nil); err != nil {
		t.Fatal(err)
	}
	if string(got) != "AABB" {
		t.Fatalf("assembled %q after restart", got)
	}
}

func TestSplitResourceDeclaredSizeTolerance(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	hash := make([]byte, 32)
	hash[0] = 0x05
	key := "aa:" + string(make([]byte, 64))

	// Peers that advertise only the current segment body in d must not be
	// rejected when their total exceeds it; the segment count bounds them.
	if err := admitSplitSegment(key, splitAdv(hash, 1, 2, 6), 4); err != nil {
		t.Fatal(err)
	}
	if err := admitSplitSegment(key, splitAdv(hash, 2, 2, 6), 3); err != nil {
		t.Fatalf("per-segment data_size peer rejected: %v", err)
	}
}

func TestSplitResourceHardAssemblyCap(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	hash := make([]byte, 32)
	hash[0] = 0x09
	key := "bb:" + string(make([]byte, 64))

	// Inflated declared totals cannot lift staging past the hard cap.
	fit := int(maxSplitAssemblyBytes / resource.MaxEfficientSize)
	for seg := uint16(1); seg <= uint16(fit); seg++ { // #nosec G115
		if err := admitSplitSegment(key, splitAdv(hash, seg, 65535, maxSplitAssemblyBytes*2), resource.MaxEfficientSize); err != nil {
			t.Fatalf("segment %d rejected before cap: %v", seg, err)
		}
	}
	if err := admitSplitSegment(key, splitAdv(hash, uint16(fit)+1, 65535, maxSplitAssemblyBytes*2), resource.MaxEfficientSize); err == nil { // #nosec G115
		t.Fatal("staged bytes beyond the hard cap were not rejected")
	}
}

func TestSplitResourceRejectsInconsistentTotals(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	l := splitTestLink()
	hash := make([]byte, 32)
	hash[0] = 0x06

	if err := l.handleSplitSegmentComplete([]byte("A"), splitAdv(hash, 1, 3, 0), nil); err != nil {
		t.Fatal(err)
	}
	if err := l.handleSplitSegmentComplete([]byte("B"), splitAdv(hash, 2, 5, 0), nil); err == nil {
		t.Fatal("segment with changed total was not rejected")
	}
}

func TestSplitResourcePerLinkAssemblyCap(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	l := splitTestLink()
	for i := 0; i < maxSplitAssembliesPerLink; i++ {
		hash := make([]byte, 32)
		hash[0] = byte(0x10 + i)
		if err := l.handleSplitSegmentComplete([]byte("x"), splitAdv(hash, 1, 2, 0), nil); err != nil {
			t.Fatalf("assembly %d rejected: %v", i, err)
		}
	}
	hash := make([]byte, 32)
	hash[0] = 0x7f
	if err := l.handleSplitSegmentComplete([]byte("x"), splitAdv(hash, 1, 2, 0), nil); err == nil {
		t.Fatal("assembly beyond per-link cap was not rejected")
	}
}

func TestDropSplitAssembliesReleasesState(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	l := splitTestLink()
	hash := make([]byte, 32)
	hash[0] = 0x08
	if err := l.handleSplitSegmentComplete([]byte("partial"), splitAdv(hash, 1, 3, 0), nil); err != nil {
		t.Fatal(err)
	}
	l.dropSplitAssemblies()

	splitResourceMu.Lock()
	nAsm := len(splitAssemblies)
	nMem := len(splitResourceMem)
	splitResourceMu.Unlock()
	if nAsm != 0 || nMem != 0 {
		t.Fatalf("teardown left %d assemblies, %d buffers", nAsm, nMem)
	}
	if splitResourceBudget.Used() != 0 {
		t.Fatalf("budget not released on teardown, used=%d", splitResourceBudget.Used())
	}
}
