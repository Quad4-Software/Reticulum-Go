// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package server

import "testing"

func BenchmarkRecordPageViewHotPath(b *testing.B) {
	r := &Reticulum{pageStats: make(map[string]int64)}
	path := "/page/index.mu"
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.recordPageView(path)
	}
}

func BenchmarkRecordPageViewDisabled(b *testing.B) {
	r := &Reticulum{pageStatsDisabled: true}
	path := "/page/index.mu"
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.recordPageView(path)
	}
}

func BenchmarkFormatPageViewStatsMicron(b *testing.B) {
	counts := map[string]int64{
		"/page/a.mu": 10,
		"/page/b.mu": 20,
		"/page/c.mu": 30,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = formatPageViewStatsMicron(1_700_000_000, counts)
	}
}

func BenchmarkServePageViewStatsCopy(b *testing.B) {
	r := &Reticulum{pageStats: map[string]int64{
		"/page/a.mu": 1,
		"/page/b.mu": 2,
		"/page/c.mu": 3,
	}}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = r.servePageViewStats("", nil, nil, nil, nil, 0)
	}
}
