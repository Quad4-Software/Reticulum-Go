// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package profiler

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProfilerCaptureAndResults(t *testing.T) {
	Reset()
	defer Reset()

	if Ran() {
		t.Fatal("expected not ran")
	}
	if Results() != nil {
		t.Fatal("expected nil results before captures")
	}

	s := Start("unit.work")
	time.Sleep(2 * time.Millisecond)
	s.End()

	if !Ran() {
		t.Fatal("expected ran")
	}
	res := Results()
	if res == nil {
		t.Fatal("expected results")
	}
	tr, ok := res["unit.work"]
	if !ok || tr.StatsAll == nil || tr.StatsAll.Count != 1 {
		t.Fatalf("bad result: %+v", res)
	}
	if tr.StatsAll.Mean == nil || *tr.StatsAll.Mean < 0.001 {
		t.Fatalf("mean too small: %v", tr.StatsAll.Mean)
	}
	text := FormatResults(res)
	if !strings.Contains(text, "unit.work") || !strings.Contains(text, "Samples") {
		t.Fatalf("format missing fields:\n%s", text)
	}
}

func TestProfilerSuperNested(t *testing.T) {
	Reset()
	defer Reset()

	outer := Start("parent")
	inner := StartSuper("child", "parent", true)
	time.Sleep(time.Millisecond)
	inner.End()
	outer.End()

	res := Results()
	if res["child"].Super == nil || *res["child"].Super != "parent" {
		t.Fatalf("super: %+v", res["child"])
	}
	text := FormatResults(res)
	if !strings.Contains(text, "parent") || !strings.Contains(text, "child") {
		t.Fatalf("nested format:\n%s", text)
	}
}

func TestProfilerDoubleEndSafe(t *testing.T) {
	Reset()
	defer Reset()
	s := Start("once")
	s.End()
	s.End()
	var nilSpan *Span
	nilSpan.End()
	Start("").End()
}

func TestProfilerConcurrent(t *testing.T) {
	Reset()
	defer Reset()
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 50 {
				Do("concurrent.tag", func() {
					time.Sleep(50 * time.Microsecond)
				})
			}
		})
	}
	wg.Wait()
	res := Results()
	tr := res["concurrent.tag"]
	if tr.StatsAll == nil || tr.StatsAll.Count < 100 {
		t.Fatalf("count=%v", tr.StatsAll)
	}
	if tr.StatsAll.Threads == nil || *tr.StatsAll.Threads < 2 {
		t.Fatalf("threads=%v", tr.StatsAll.Threads)
	}
}

func TestProfilerMaxCaptures(t *testing.T) {
	Reset()
	defer Reset()
	for range MaxCaptures + 100 {
		Do("ring", func() {})
	}
	res := Results()
	if res["ring"].StatsAll.Count > MaxCaptures {
		t.Fatalf("count %d exceeds max", res["ring"].StatsAll.Count)
	}
}
