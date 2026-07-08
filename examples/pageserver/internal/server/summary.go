// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package server

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// PrintStartupSummary writes node hash, paths, and timing hints to stderr.
func PrintStartupSummary(r *Reticulum) {
	h := fmt.Sprintf("%x", r.destination.GetHash())
	pages := make([]string, 0, len(r.registeredPagePaths))
	for p := range r.registeredPagePaths {
		pages = append(pages, p)
	}
	sort.Strings(pages)
	files := make([]string, 0, len(r.registeredFilePaths))
	for p := range r.registeredFilePaths {
		files = append(files, p)
	}
	sort.Strings(files)

	w := os.Stderr
	fmt.Fprintf(w, "pageserver node destination hash: %s\n", h)
	fmt.Fprintf(w, "  node name: %s\n", r.config.AppName)
	fmt.Fprintf(w, "  pages (%d): ", len(pages))
	if len(pages) == 0 {
		fmt.Fprintln(w, "(none)")
	} else {
		fmt.Fprintln(w, strings.Join(pages, ", "))
	}
	fmt.Fprintf(w, "  files (%d): ", len(files))
	if len(files) == 0 {
		fmt.Fprintln(w, "(none)")
	} else {
		fmt.Fprintln(w, strings.Join(files, ", "))
	}
	if r.pageStatsDisabled {
		fmt.Fprintln(w, "  page view stats: off")
	} else {
		fmt.Fprintf(w, "  page view stats: %s\n", BuiltInPageViewsPath)
	}
	if r.announceEveryMinutes > 0 {
		ann := time.Duration(r.announceEveryMinutes) * time.Minute
		fmt.Fprintf(w, "  periodic announce: every %d min (%s)\n", r.announceEveryMinutes, FormatDuration(ann))
	} else {
		fmt.Fprintln(w, "  periodic announce: off (initial announce only)")
	}
	if r.pagesRefreshInterval > 0 {
		sec := int(r.pagesRefreshInterval / time.Second)
		fmt.Fprintf(w, "  pages rescan: every %ds (%s)\n", sec, FormatDuration(r.pagesRefreshInterval))
	}
	if r.filesRefreshInterval > 0 {
		sec := int(r.filesRefreshInterval / time.Second)
		fmt.Fprintf(w, "  files rescan: every %ds (%s)\n", sec, FormatDuration(r.filesRefreshInterval))
	}
}
