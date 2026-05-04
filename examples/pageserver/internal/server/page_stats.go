// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package server

import (
	"fmt"
	"maps"
	"math"
	"sort"
	"strings"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
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
	b.WriteString("`c`!Page view statistics`!\n\n")
	b.WriteString("`F888Successful page responses only.`f\n\n")
	fmt.Fprintf(&b, "`!Total views`! `!%d`!\n\n", total)
	fmt.Fprintf(&b, "`!Generated (unix)`! `!%d`!\n\n", generatedUnix)
	b.WriteString("-\n\n")
	b.WriteString("`!Per path`!\n\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "%s\n", escapeMicronPath(p))
		fmt.Fprintf(&b, "`!%d`! views\n\n", counts[p])
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
	r.pageStatsMu.Lock()
	defer r.pageStatsMu.Unlock()
	if r.pageStats == nil {
		r.pageStats = make(map[string]int64)
	}
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
	r.pageStatsMu.Lock()
	snap := make(map[string]int64, len(r.pageStats))
	maps.Copy(snap, r.pageStats)
	r.pageStatsMu.Unlock()

	return formatPageViewStatsMicron(time.Now().Unix(), snap)
}

func (r *Reticulum) syncBuiltInPageStatsHandler() {
	if err := r.destination.RegisterRequestHandler(
		BuiltInPageViewsPath,
		r.servePageViewStats,
		destination.AllowAll,
		nil,
	); err != nil {
		debug.Log(debug.DebugError, "Failed to register page view stats handler", "path", BuiltInPageViewsPath, "error", err)
	}
}
