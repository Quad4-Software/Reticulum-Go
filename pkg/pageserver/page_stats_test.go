// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package pageserver

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestFormatPageViewStatsMicronOrderAndTotal(t *testing.T) {
	b := formatPageViewStatsMicron(99, map[string]int64{
		"/page/b.mu": 2,
		"/page/a.mu": 5,
	})
	s := string(b)
	if !strings.Contains(s, "`!Generated (unix)`! `!99`!") {
		t.Fatalf("missing generated line: %q", s)
	}
	if !strings.Contains(s, "`!Total views`! `!7`!") {
		t.Fatalf("missing total 7: %q", s)
	}
	ia := strings.Index(s, "/page/a.mu")
	ib := strings.Index(s, "/page/b.mu")
	if ia < 0 || ib < 0 {
		t.Fatalf("missing path lines: %q", s)
	}
	if ia >= ib {
		t.Fatalf("want /page/a before /page/b, got ia=%d ib=%d", ia, ib)
	}
}

func TestSumPageViewCountsSaturated(t *testing.T) {
	if got := sumPageViewCountsSaturated(map[string]int64{"a": 10, "b": 20}); got != 30 {
		t.Fatalf("got %d", got)
	}
	if got := sumPageViewCountsSaturated(map[string]int64{
		"a": math.MaxInt64 - 5,
		"b": 10,
	}); got != math.MaxInt64 {
		t.Fatalf("overflow sum: got %d want MaxInt64", got)
	}
}

func TestFormatPageViewStatsMicronTotalMatchesSaturatedSum(t *testing.T) {
	counts := map[string]int64{
		"/x": math.MaxInt64 / 3,
		"/y": math.MaxInt64 / 3,
		"/z": math.MaxInt64 / 3,
	}
	b := formatPageViewStatsMicron(1, counts)
	want := sumPageViewCountsSaturated(counts)
	s := string(b)
	if !strings.Contains(s, fmt.Sprintf("`!Total views`! `!%d`!", want)) {
		t.Fatalf("micron total line missing %d in: %s", want, b)
	}
}

func TestRecordPageViewSaturatesPerPath(t *testing.T) {
	prev := PageStatsMaxCountPerPath
	PageStatsMaxCountPerPath = 3
	defer func() { PageStatsMaxCountPerPath = prev }()

	r := &Reticulum{pageStats: make(map[string]int64)}
	for range 20 {
		r.recordPageView("/page/a.mu")
	}
	r.pageStatsMu.Lock()
	got := r.pageStats["/page/a.mu"]
	r.pageStatsMu.Unlock()
	if got != 3 {
		t.Fatalf("per-path cap: got %d want 3", got)
	}
}

func TestRecordPageViewEvictsWhenDistinctPathsExceedCap(t *testing.T) {
	prev := PageStatsMaxPaths
	PageStatsMaxPaths = 3
	defer func() { PageStatsMaxPaths = prev }()

	r := &Reticulum{pageStats: make(map[string]int64)}
	r.recordPageView("/page/1.mu")
	r.recordPageView("/page/2.mu")
	r.recordPageView("/page/3.mu")
	r.recordPageView("/page/4.mu")

	r.pageStatsMu.Lock()
	n := len(r.pageStats)
	_, has4 := r.pageStats["/page/4.mu"]
	r.pageStatsMu.Unlock()
	if n != 3 {
		t.Fatalf("map size: got %d want 3", n)
	}
	if !has4 {
		t.Fatal("expected newest path /page/4.mu to remain after eviction")
	}
}

func TestRecordPageViewExistingPathsNoEvictionWhenAtCap(t *testing.T) {
	prev := PageStatsMaxPaths
	PageStatsMaxPaths = 2
	defer func() { PageStatsMaxPaths = prev }()

	r := &Reticulum{pageStats: make(map[string]int64)}
	r.recordPageView("/page/a.mu")
	r.recordPageView("/page/b.mu")
	r.recordPageView("/page/a.mu")
	r.recordPageView("/page/b.mu")

	r.pageStatsMu.Lock()
	n := len(r.pageStats)
	va := r.pageStats["/page/a.mu"]
	vb := r.pageStats["/page/b.mu"]
	r.pageStatsMu.Unlock()
	if n != 2 {
		t.Fatalf("map size: got %d want 2", n)
	}
	if va != 2 || vb != 2 {
		t.Fatalf("counts a=%d b=%d", va, vb)
	}
}

func TestServePageViewStatsRoundTrip(t *testing.T) {
	r := &Reticulum{pageStats: map[string]int64{"/page/z.mu": 2, "/page/a.mu": 1}}
	out := r.servePageViewStats("", nil, nil, nil, nil, 0)
	s := string(out)
	if !strings.Contains(s, "`!Total views`! `!3`!") {
		t.Fatalf("total: %q", s)
	}
	if !strings.Contains(s, "/page/a.mu") || !strings.Contains(s, "/page/z.mu") {
		t.Fatalf("paths: %q", s)
	}
	if !strings.Contains(s, "`!1`! views") || !strings.Contains(s, "`!2`! views") {
		t.Fatalf("per-path counts: %q", s)
	}
}

func TestEscapeMicronPath(t *testing.T) {
	if got := escapeMicronPath("/page/`x`.mu"); got != "/page/'x'.mu" {
		t.Fatalf("got %q", got)
	}
}

func TestRecordPageViewSkipsWhenStatsDisabled(t *testing.T) {
	r := &Reticulum{pageStatsDisabled: true, pageStats: nil}
	r.recordPageView("/page/x.mu")
	if r.pageStats != nil {
		t.Fatalf("expected nil map when stats disabled, got %v", r.pageStats)
	}
}
