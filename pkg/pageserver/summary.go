// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package pageserver

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/term"
)

// PrintStartupSummary writes node hash, paths, and timing hints to stderr.
func PrintStartupSummary(r *Reticulum) {
	PrintStartupSummaryTo(os.Stderr, r)
}

// PrintStartupSummaryTo writes the startup banner to w.
func PrintStartupSummaryTo(w io.Writer, r *Reticulum) {
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

	fmt.Fprintf(w, "%s %s\n", term.BoldW(w, "pageserver node destination hash:"), term.CyanW(w, h))
	fmt.Fprintf(w, "  node name: %s\n", r.config.AppName)
	fmt.Fprintf(w, "  pages (%d): ", len(pages))
	if len(pages) == 0 {
		fmt.Fprintln(w, term.YellowW(w, "(none)"))
	} else {
		fmt.Fprintln(w, strings.Join(pages, ", "))
	}
	fmt.Fprintf(w, "  files (%d): ", len(files))
	if len(files) == 0 {
		fmt.Fprintln(w, term.YellowW(w, "(none)"))
	} else {
		fmt.Fprintln(w, strings.Join(files, ", "))
	}
	if r.pageStatsDisabled {
		fmt.Fprintln(w, "  page view stats:", term.YellowW(w, "off"))
	} else {
		fmt.Fprintf(w, "  page view stats: %s\n", BuiltInPageViewsPath)
	}
	if r.announceEveryMinutes > 0 {
		ann := time.Duration(r.announceEveryMinutes) * time.Minute
		fmt.Fprintf(w, "  periodic announce: every %d min (%s)\n", r.announceEveryMinutes, FormatDuration(ann))
	} else {
		fmt.Fprintln(w, "  periodic announce:", term.YellowW(w, "off (initial announce only)"))
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
