// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package server

import (
	"maps"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

// BuiltInPageViewsPath is reserved. It overrides any file at pages/__pageviews.mu after sync.
const BuiltInPageViewsPath = "/page/__pageviews.mu"

// Tunable caps (tests may lower these).
var (
	PageStatsMaxPaths        = 4096
	PageStatsMaxCountPerPath = int64(1 << 62)
)

func sumPageViewCountsSaturated(counts map[string]int64) int64 {
	var total int64
	for _, c := range counts {
		if c <= 0 {
			continue
		}
		if total > math.MaxInt64-c {
			return math.MaxInt64
		}
		total += c
	}
	return total
}

func escapeMicronPath(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}

// formatPageViewStatsMicron returns a .mu body in Nomad-style Micron markup.
func formatPageViewStatsMicron(generatedUnix int64, counts map[string]int64) []byte {
	paths := make([]string, 0, len(counts))
	for p := range counts {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	total := sumPageViewCountsSaturated(counts)

	var b strings.Builder
	b.Grow(256 + len(paths)*96)
	b.WriteString("`c`!Page view statistics`!\n\n")
	b.WriteString("`F888Successful page responses only.`f\n\n")
	b.WriteString("`!Total views`! `!")
	var nbuf [24]byte
	b.Write(strconv.AppendInt(nbuf[:0], total, 10))
	b.WriteString("`!\n\n")
	b.WriteString("`!Generated (unix)`! `!")
	b.Write(strconv.AppendInt(nbuf[:0], generatedUnix, 10))
	b.WriteString("`!\n\n")
	b.WriteString("-\n\n")
	b.WriteString("`!Per path`!\n\n")
	for _, p := range paths {
		b.WriteString(escapeMicronPath(p))
		b.WriteByte('\n')
		b.WriteString("`!")
		b.Write(strconv.AppendInt(nbuf[:0], counts[p], 10))
		b.WriteString("`! views\n\n")
	}
	return []byte(b.String())
}

func (r *Reticulum) evictOnePageStatLocked() {
	for k := range r.pageStats {
		delete(r.pageStats, k)
		return
	}
}

func (r *Reticulum) recordPageView(requestPath string) {
	if r.pageStatsDisabled {
		return
	}
	r.pageStatsMu.Lock()
	defer r.pageStatsMu.Unlock()
	v, ok := r.pageStats[requestPath]
	if ok {
		if v < PageStatsMaxCountPerPath {
			r.pageStats[requestPath] = v + 1
		}
		return
	}
	if len(r.pageStats) >= PageStatsMaxPaths {
		r.evictOnePageStatLocked()
	}
	r.pageStats[requestPath] = 1
}

func (r *Reticulum) servePageViewStats(
	_ string,
	_ []byte,
	_ []byte,
	_ []byte,
	_ *identity.Identity,
	_ int64,
) []byte {
	r.pageStatsMu.RLock()
	n := len(r.pageStats)
	snap := make(map[string]int64, n)
	if n > 0 {
		maps.Copy(snap, r.pageStats)
	}
	r.pageStatsMu.RUnlock()

	return formatPageViewStatsMicron(time.Now().Unix(), snap)
}

func (r *Reticulum) syncBuiltInPageStatsHandler() {
	if r.pageStatsDisabled {
		return
	}
	if err := r.destination.RegisterRequestHandler(
		BuiltInPageViewsPath,
		r.servePageViewStats,
		destination.AllowAll,
		nil,
	); err != nil {
		debug.Log(debug.DebugError, "Failed to register page view stats handler", "path", BuiltInPageViewsPath, "error", err)
	}
}
