// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"errors"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/resource"
	"quad4/reticulum-go/pkg/transport"
)

func TestSplitResourceInMemoryBudgetExceeded(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	cfg := &common.ReticulumConfig{
		InMemoryStorage:          true,
		MaxInMemoryResourceBytes: 32,
		ShareInstance:            false,
	}
	tr := transport.NewTransport(cfg)
	l := &Link{transport: tr}

	hash := make([]byte, 32)
	hash[0] = 0xab
	adv := &resource.ResourceAdvertisement{
		OriginalHash:  hash,
		SegmentIndex:  1,
		TotalSegments: 2,
	}
	payload := make([]byte, 40)
	err := l.handleSplitSegmentComplete(payload, adv)
	if !errors.Is(err, common.ErrMemoryBudgetExceeded) {
		t.Fatalf("expected ErrMemoryBudgetExceeded, got %v", err)
	}
	if splitResourceBudget.Used() != 0 {
		t.Fatalf("failed reserve must not charge budget, used=%d", splitResourceBudget.Used())
	}
}

func TestSplitResourceInMemoryRoundTrip(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	cfg := &common.ReticulumConfig{
		InMemoryStorage:          true,
		MaxInMemoryResourceBytes: 1 << 20,
		ShareInstance:            false,
	}
	tr := transport.NewTransport(cfg)
	var got []byte
	l := &Link{
		transport: tr,
		linkID:    []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00},
		resourceConcludedCallback: func(v any) {
			switch x := v.(type) {
			case []byte:
				got = append([]byte(nil), x...)
			case IncomingResource:
				got = append([]byte(nil), x.Data...)
			}
		},
	}

	hash := make([]byte, 32)
	hash[0] = 0xcd
	seg1 := []byte("hello ")
	seg2 := []byte("world")
	adv1 := &resource.ResourceAdvertisement{
		OriginalHash:  hash,
		SegmentIndex:  1,
		TotalSegments: 2,
	}
	adv2 := &resource.ResourceAdvertisement{
		OriginalHash:  hash,
		SegmentIndex:  2,
		TotalSegments: 2,
	}
	if err := l.handleSplitSegmentComplete(seg1, adv1); err != nil {
		t.Fatal(err)
	}
	wantKey := l.splitResourceKey(hash)
	splitResourceMu.Lock()
	_, pending := splitResourceMem[wantKey]
	splitResourceMu.Unlock()
	if !pending {
		t.Fatal("expected staged segment before final")
	}
	if err := l.handleSplitSegmentComplete(seg2, adv2); err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Fatalf("got %q", got)
	}
	if splitResourceBudget.Used() != 0 {
		t.Fatalf("budget should release after complete, used=%d", splitResourceBudget.Used())
	}
}

func TestSplitResourceInMemoryIsolatesLinks(t *testing.T) {
	resetSplitResourceMemoryForTest()
	t.Cleanup(resetSplitResourceMemoryForTest)

	cfg := &common.ReticulumConfig{
		InMemoryStorage:          true,
		MaxInMemoryResourceBytes: 1 << 20,
		ShareInstance:            false,
	}
	tr := transport.NewTransport(cfg)
	hash := make([]byte, 32)
	hash[0] = 0xee

	var gotA, gotB []byte
	a := &Link{
		transport: tr,
		linkID:    []byte{0xa1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		resourceConcludedCallback: func(v any) {
			if b, ok := v.([]byte); ok {
				gotA = append([]byte(nil), b...)
			}
		},
	}
	b := &Link{
		transport: tr,
		linkID:    []byte{0xb2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
		resourceConcludedCallback: func(v any) {
			if x, ok := v.([]byte); ok {
				gotB = append([]byte(nil), x...)
			}
		},
	}

	adv1 := &resource.ResourceAdvertisement{OriginalHash: hash, SegmentIndex: 1, TotalSegments: 2}
	adv2 := &resource.ResourceAdvertisement{OriginalHash: hash, SegmentIndex: 2, TotalSegments: 2}
	if err := a.handleSplitSegmentComplete([]byte("A1"), adv1); err != nil {
		t.Fatal(err)
	}
	if err := b.handleSplitSegmentComplete([]byte("B1"), adv1); err != nil {
		t.Fatal(err)
	}
	if err := a.handleSplitSegmentComplete([]byte("A2"), adv2); err != nil {
		t.Fatal(err)
	}
	if err := b.handleSplitSegmentComplete([]byte("B2"), adv2); err != nil {
		t.Fatal(err)
	}
	if string(gotA) != "A1A2" {
		t.Fatalf("link A got %q", gotA)
	}
	if string(gotB) != "B1B2" {
		t.Fatalf("link B got %q", gotB)
	}
}
