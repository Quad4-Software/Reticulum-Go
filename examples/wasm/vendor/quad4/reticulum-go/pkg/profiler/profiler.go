// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package profiler provides RNS.Profiler-compatible live timing capture for
// shared-instance RPC (get profiling_results) and remote /status.
package profiler

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// MaxCaptures matches Python RNS.Profiler.MAX_CAPTURES.
	MaxCaptures = 10000
)

type capture struct {
	start       float64
	end         float64
	threadIdent uint64
	done        bool
}

type tagState struct {
	mu       sync.Mutex
	super    string
	hasSuper bool
	captures []capture
	running  map[uint64][]int
}

var (
	globalMu  sync.Mutex
	tags      = map[string]*tagState{}
	ranAtomic atomic.Bool
	gidSeq    atomic.Uint64
)

// Span is an open capture. Call End when finished.
type Span struct {
	tag string
	id  uint64
}

// Start begins a capture for tag.
func Start(tag string) *Span {
	return StartSuper(tag, "", false)
}

// StartSuper begins a capture nested under superTag when hasSuper is true.
func StartSuper(tag, superTag string, hasSuper bool) *Span {
	if tag == "" {
		return nil
	}
	st := getTag(tag, superTag, hasSuper)
	now := float64(time.Now().UnixNano()) / 1e9
	id := gidSeq.Add(1)
	st.mu.Lock()
	st.push(capture{start: now, threadIdent: id})
	idx := len(st.captures) - 1
	st.running[id] = append(st.running[id], idx)
	st.mu.Unlock()
	return &Span{tag: tag, id: id}
}

// End finishes the capture.
func (s *Span) End() {
	if s == nil || s.tag == "" {
		return
	}
	st := lookupTag(s.tag)
	if st == nil {
		return
	}
	now := float64(time.Now().UnixNano()) / 1e9
	st.mu.Lock()
	defer st.mu.Unlock()
	stack := st.running[s.id]
	if len(stack) == 0 {
		return
	}
	idx := stack[len(stack)-1]
	st.running[s.id] = stack[:len(stack)-1]
	if len(st.running[s.id]) == 0 {
		delete(st.running, s.id)
	}
	if idx < 0 || idx >= len(st.captures) {
		return
	}
	c := &st.captures[idx]
	if c.done {
		return
	}
	c.end = now
	c.done = true
	ranAtomic.Store(true)
}

// Do runs fn under tag.
func Do(tag string, fn func()) {
	s := Start(tag)
	defer s.End()
	fn()
}

func getTag(tag, super string, hasSuper bool) *tagState {
	globalMu.Lock()
	defer globalMu.Unlock()
	st, ok := tags[tag]
	if !ok {
		st = &tagState{
			super:    super,
			hasSuper: hasSuper,
			captures: make([]capture, 0, 64),
			running:  make(map[uint64][]int),
		}
		tags[tag] = st
		return st
	}
	if hasSuper && !st.hasSuper {
		st.super = super
		st.hasSuper = true
	}
	return st
}

func lookupTag(tag string) *tagState {
	globalMu.Lock()
	defer globalMu.Unlock()
	return tags[tag]
}

func (st *tagState) push(c capture) {
	if len(st.captures) >= MaxCaptures {
		// Drop oldest like Python deque(maxlen=). Adjust running indices.
		st.captures = st.captures[1:]
		for id, stack := range st.running {
			out := stack[:0]
			for _, idx := range stack {
				if idx == 0 {
					continue
				}
				out = append(out, idx-1)
			}
			if len(out) == 0 {
				delete(st.running, id)
			} else {
				st.running[id] = out
			}
		}
	}
	st.captures = append(st.captures, c)
}

// Ran reports whether any capture has completed (Python Profiler.ran).
func Ran() bool {
	return ranAtomic.Load()
}

// Reset clears all captures. Tests only.
func Reset() {
	globalMu.Lock()
	defer globalMu.Unlock()
	tags = map[string]*tagState{}
	ranAtomic.Store(false)
}

// Stats is one timing window summary matching Python calc_stats output.
type Stats struct {
	Count   int      `msgpack:"count" json:"count"`
	Mean    *float64 `msgpack:"mean" json:"mean"`
	Median  *float64 `msgpack:"median" json:"median"`
	Min     *float64 `msgpack:"min" json:"min"`
	Max     *float64 `msgpack:"max" json:"max"`
	Stdev   *float64 `msgpack:"stdev" json:"stdev"`
	Sum     *float64 `msgpack:"sum" json:"sum"`
	Threads *int     `msgpack:"threads" json:"threads"`
}

// TagResult is one profiler's aggregated windows (Python Profiler.results row).
type TagResult struct {
	Name     string  `msgpack:"name" json:"name"`
	Super    *string `msgpack:"super" json:"super"`
	StatsAll *Stats  `msgpack:"stats_all" json:"stats_all"`
	Stats1m  *Stats  `msgpack:"stats_1m" json:"stats_1m"`
	Stats5m  *Stats  `msgpack:"stats_5m" json:"stats_5m"`
	Stats30m *Stats  `msgpack:"stats_30m" json:"stats_30m"`
	Stats60m *Stats  `msgpack:"stats_60m" json:"stats_60m"`
}

// Results returns the Python-compatible results map keyed by tag name.
// Returns nil when no captures have completed.
func Results() map[string]TagResult {
	if !Ran() {
		return nil
	}
	now := float64(time.Now().UnixNano()) / 1e9
	globalMu.Lock()
	names := make([]string, 0, len(tags))
	for name := range tags {
		names = append(names, name)
	}
	sort.Strings(names)
	type snap struct {
		st       *tagState
		super    string
		hasSuper bool
	}
	snaps := make([]snap, len(names))
	for i, name := range names {
		st := tags[name]
		snaps[i] = snap{st: st, super: st.super, hasSuper: st.hasSuper}
	}
	globalMu.Unlock()

	out := make(map[string]TagResult, len(names))
	for i, name := range names {
		st := snaps[i].st
		st.mu.Lock()
		caps := append([]capture(nil), st.captures...)
		st.mu.Unlock()

		completed := make([]capture, 0, len(caps))
		for _, c := range caps {
			if c.done && c.end >= c.start {
				completed = append(completed, c)
			}
		}
		if len(completed) == 0 {
			continue
		}
		sort.SliceStable(completed, func(a, b int) bool {
			return completed[a].start < completed[b].start
		})

		all := calcStats(completed, 0, len(completed), true)
		if all == nil || all.Count == 0 {
			continue
		}
		tr := TagResult{
			Name:     name,
			StatsAll: all,
			Stats1m:  windowStats(completed, now, 60, 0),
			Stats5m:  windowStats(completed, now, 5*60, 60),
			Stats30m: windowStats(completed, now, 30*60, 5*60),
			Stats60m: windowStats(completed, now, 60*60, 30*60),
		}
		if snaps[i].hasSuper {
			s := snaps[i].super
			tr.Super = &s
		}
		out[name] = tr
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func windowStats(caps []capture, now, outer, inner float64) *Stats {
	lo := now - outer
	hi := now
	if inner > 0 {
		hi = now - inner
	}
	start := sort.Search(len(caps), func(i int) bool {
		return caps[i].start >= lo
	})
	end := sort.Search(len(caps), func(i int) bool {
		return caps[i].start >= hi
	})
	if start >= end {
		return nil
	}
	return calcStats(caps, start, end, false)
}

func calcStats(caps []capture, start, end int, withThreads bool) *Stats {
	if start < 0 {
		start = 0
	}
	if end > len(caps) {
		end = len(caps)
	}
	if start >= end {
		return nil
	}
	durs := make([]float64, 0, end-start)
	uniq := make(map[uint64]struct{})
	for i := start; i < end; i++ {
		c := caps[i]
		if !c.done {
			continue
		}
		d := c.end - c.start
		if d < 0 {
			continue
		}
		durs = append(durs, d)
		if withThreads {
			uniq[c.threadIdent] = struct{}{}
		}
	}
	n := len(durs)
	if n == 0 {
		return nil
	}
	sort.Float64s(durs)
	sum := 0.0
	for _, d := range durs {
		sum += d
	}
	mean := sum / float64(n)
	var median float64
	if n%2 == 1 {
		median = durs[n/2]
	} else {
		median = (durs[n/2-1] + durs[n/2]) / 2
	}
	minV, maxV := durs[0], durs[n-1]
	var stdev *float64
	if n > 1 {
		var ck, ck2 float64
		for _, d := range durs {
			diff := d - median
			ck += diff
			ck2 += diff * diff
		}
		s := math.Sqrt((ck2 - (ck*ck)/float64(n)) / float64(n-1))
		stdev = &s
	}
	st := &Stats{
		Count:  n,
		Mean:   &mean,
		Median: &median,
		Min:    &minV,
		Max:    &maxV,
		Stdev:  stdev,
		Sum:    &sum,
	}
	if withThreads {
		t := len(uniq)
		st.Threads = &t
	}
	return st
}

// FormatResults matches Python RNS.Profiler.format_results for human display.
func FormatResults(results map[string]TagResult) string {
	if len(results) == 0 {
		return ""
	}
	names := make([]string, 0, len(results))
	for name, tr := range results {
		if tr.Super == nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		b.WriteString(formatRecursive(results[name], results, 0))
	}
	return b.String()
}

func formatRecursive(tag TagResult, all map[string]TagResult, level int) string {
	var s strings.Builder
	s.WriteString(formatTag(tag, level+1) + "\n")
	children := make([]string, 0)
	for name, sub := range all {
		if sub.Super != nil && *sub.Super == tag.Name {
			children = append(children, name)
		}
	}
	sort.Strings(children)
	for _, name := range children {
		s.WriteString(formatRecursive(all[name], all, level+1))
	}
	return s.String()
}

func formatTag(tag TagResult, level int) string {
	ind := strings.Repeat("  ", level)
	var b strings.Builder
	fmt.Fprintf(&b, " %s%s\n", ind, tag.Name)
	if tag.StatsAll != nil {
		threads := 0
		if tag.StatsAll.Threads != nil {
			threads = *tag.StatsAll.Threads
		}
		plural := ""
		if threads != 1 {
			plural = "s"
		}
		fmt.Fprintf(&b, " %s  Samples  : %d from %d thread%s\n", ind, tag.StatsAll.Count, threads, plural)
		fmt.Fprintf(&b, " %s              %-15s | %-15s | %-15s | %-15s | %-15s | %-15s\n",
			ind, "Mean", "Median", "Min", "Max", "St. dev", "Total")
		fmt.Fprintf(&b, " %s  Stats    : (%s | %s | %s | %s | %s | %s)\n",
			ind, pst(tag.StatsAll.Mean), pst(tag.StatsAll.Median), pst(tag.StatsAll.Min),
			pst(tag.StatsAll.Max), pst(tag.StatsAll.Stdev), pst(tag.StatsAll.Sum))
	}
	writeWindow(&b, ind, "   0-1m    ", tag.Stats1m)
	writeWindow(&b, ind, "   1-5m    ", tag.Stats5m)
	writeWindow(&b, ind, "  5-30m    ", tag.Stats30m)
	writeWindow(&b, ind, " 30-60m    ", tag.Stats60m)
	return b.String()
}

func writeWindow(b *strings.Builder, ind, label string, st *Stats) {
	if st == nil {
		return
	}
	fmt.Fprintf(b, " %s%s: (%s | %s | %s | %s | %s | %s)\n",
		ind, label, pst(st.Mean), pst(st.Median), pst(st.Min), pst(st.Max), pst(st.Stdev), pst(st.Sum))
}

func pst(v *float64) string {
	if v == nil {
		return "-----"
	}
	return prettyShortTime(*v)
}

func prettyShortTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	switch {
	case sec < 1e-6:
		return fmt.Sprintf("%.0fns", sec*1e9)
	case sec < 1e-3:
		return fmt.Sprintf("%.1fµs", sec*1e6)
	case sec < 1:
		return fmt.Sprintf("%.1fms", sec*1e3)
	case sec < 60:
		return fmt.Sprintf("%.2fs", sec)
	default:
		return fmt.Sprintf("%.1fm", sec/60)
	}
}

// ResultsOrNil returns Results for RPC nil when nothing has run.
func ResultsOrNil() any {
	r := Results()
	if r == nil {
		return nil
	}
	return r
}
