// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"encoding/hex"
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
	key := hex.EncodeToString(hash)
	splitResourceMu.Lock()
	_, pending := splitResourceMem[key]
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
