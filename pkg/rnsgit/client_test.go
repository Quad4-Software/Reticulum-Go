// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"testing"
)

type mockBatchLink struct {
	rtt float64
}

func (m mockBatchLink) GetRTT() float64 { return m.rtt }

type mockBatchTransport struct {
	bitrate int64
}

func (m mockBatchTransport) SlowestOnlineBitrate() int64 { return m.bitrate }

func TestDynamicRefBatchRTT(t *testing.T) {
	base := 25
	slow := dynamicRefBatch(base, mockBatchLink{rtt: 3.0}, nil)
	if slow != 12 {
		t.Fatalf("slow RTT batch = %d, want 12", slow)
	}
	fast := dynamicRefBatch(base, mockBatchLink{rtt: 0.1}, nil)
	if fast != 50 {
		t.Fatalf("fast RTT batch = %d, want 50", fast)
	}
}

func TestDynamicRefBatchBitrate(t *testing.T) {
	base := 16
	low := dynamicRefBatch(base, nil, mockBatchTransport{bitrate: 1200})
	if low != 8 {
		t.Fatalf("low bitrate batch = %d, want 8", low)
	}
	high := dynamicRefBatch(base, nil, mockBatchTransport{bitrate: 20_000_000})
	if high != 32 {
		t.Fatalf("high bitrate batch = %d, want 32", high)
	}
}

func TestDynamicRefBatchCap(t *testing.T) {
	got := dynamicRefBatch(40, mockBatchLink{rtt: 0.05}, mockBatchTransport{bitrate: 50_000_000})
	if got != 64 {
		t.Fatalf("capped batch = %d, want 64", got)
	}
}

func TestParsePermsGetBody(t *testing.T) {
	body := PermsGetResponse("alice: admin\n")
	got, err := ParsePermsGetBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if got != "alice: admin\n" {
		t.Fatalf("content = %q", got)
	}
}
