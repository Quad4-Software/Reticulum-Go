// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"fmt"
	"strings"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

// Chart colors for the stats page, chosen to read on dark terminals.
const (
	chartColorView     = "79c0ff"
	chartColorFetch    = "56d364"
	chartColorPush     = "ff9d5c"
	chartColorDownload = "d2a8ff"
)

// serveStatsPage renders the repository activity page, gated by the stats
// permission.
func (n *Node) serveStatsPage(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return n.renderTemplate(string(errBody), "", "stats", st)
	}
	_ = repoPath
	hash := remotePageHash(remote)
	if !n.accessTable().Resolve(group, repo, hash, permStats) {
		return n.renderTemplate(pageError("Statistics are not available for this repository"), "", "stats", st)
	}
	stats := n.repositoryStats(group, repo, statsLookbackDays)
	nav := n.repoNav(group, repo) + " / " + mLink("Stats", pagePathStats,
		map[string]string{"g": group, "r": repo})

	body := func(b *strings.Builder) {
		b.WriteString(mHeading(3) + " Repository activity\n\n")
		b.WriteString(mDim("Range: "+stats.DateRange+" ("+fmtInt(stats.LookbackDays)+" days)") + "\n\n")

		levelColor := map[string]string{
			"inactive": "888", "low": "58f", "moderate": "fc2", "high": "4d4",
		}[stats.ActivityLevel]
		b.WriteString("Activity: " + mFg(stats.ActivityLevel, levelColor) +
			"  " + mDim("score "+fmtInt(stats.ActivityScore)+", observed over "+fmtInt(stats.ActualDays)+" days") + "\n\n")

		totals := [][]string{
			{"Views", fmtInt64(stats.Series[statView].Total)},
			{"Fetches", fmtInt64(stats.Series[statFetch].Total)},
			{"Pushes", fmtInt64(stats.Series[statPush].Total)},
			{"Downloads", fmtInt64(stats.DownloadsTotal.Total)},
		}
		b.WriteString("`=\n")
		for _, row := range totals {
			fmt.Fprintf(b, "%-12s %8s\n", row[0], row[1])
		}
		b.WriteString("`=\n\n")

		b.WriteString(mHeading(4) + " Combined activity\n\n")
		b.WriteString(renderCombinedChart(
			stats.Series[statView].Daily,
			stats.Series[statFetch].Daily,
			stats.Series[statPush].Daily,
			stats.DownloadsTotal.Daily,
			stats.DayLabels, 6))

		sections := []struct {
			title, kind, color string
		}{
			{"Page views", statView, chartColorView},
			{"Fetches", statFetch, chartColorFetch},
			{"Pushes", statPush, chartColorPush},
			{"Downloads", statDownload, chartColorDownload},
		}
		for _, s := range sections {
			ser := stats.Series[s.kind]
			b.WriteString("\n" + mHeading(4) + " " + s.title + "\n\n")
			b.WriteString(mDim("Total " + fmtInt64(ser.Total)))
			if ser.PeakDay != "" {
				b.WriteString(mDim(", peak " + fmtInt64(ser.Peak) + " on " + ser.PeakDay))
			}
			b.WriteString("\n")
			b.WriteString(renderHalfblockChart(ser.Daily, stats.DayLabels, s.color, 6))
		}
	}
	return n.renderPage("stats", nav, st, body)
}

func fmtInt64(n int64) string { return fmt.Sprintf("%d", n) }

// expandHex expands a 3-digit hex color to 6 digits.
func expandHex(c string) string {
	if len(c) == 3 {
		return string([]byte{c[0], c[0], c[1], c[1], c[2], c[2]})
	}
	if len(c) > 6 {
		return c[:6]
	}
	return c
}

func hexToRGB(h string) (int, int, int) {
	var r, g, b int
	fmt.Sscanf(h, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

// gradientColor interpolates from secondary to primary by t in [0,1] scaled
// by factor.
func gradientColor(primary, secondary string, t, factor float64) string {
	pr, pg, pb := hexToRGB(primary)
	sr, sg, sb := hexToRGB(secondary)
	v := t * factor
	if v > 1 {
		v = 1
	}
	mix := func(p, s int) int { return int(float64(s) + (float64(p)-float64(s))*v) }
	return fmt.Sprintf("%02x%02x%02x", mix(pr, sr), mix(pg, sg), mix(pb, sb))
}

// chartFooter renders the shared axis line and range labels.
func chartFooter(width int, labels []string) string {
	if width < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("└" + strings.Repeat("─", width) + "┘\n")
	if len(labels) >= 2 {
		first := truncate(labels[0], 12)
		last := truncate(labels[len(labels)-1], 12)
		pad := width + 2 - len(first) - len(last)
		if pad < 0 {
			pad = 0
		}
		b.WriteString("`F666" + first + strings.Repeat(" ", pad) + last + "`f\n")
	}
	return b.String()
}

// renderHalfblockChart renders a single-series bar chart using half-block
// characters with a vertical color gradient. Semantics match the reference
// halfblock renderer: each column encodes two samples of bar height per
// terminal row.
func renderHalfblockChart(data []int64, labels []string, color string, height int) string {
	if len(data) == 0 || allZero(data) {
		return "No data available\n"
	}
	var maxVal int64
	for _, v := range data {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}
	primary := expandHex(color)
	pr, pg, pb := hexToRGB(primary)
	secondary := fmt.Sprintf("%02x%02x%02x", int(float64(pr)*0.42), int(float64(pg)*0.42), int(float64(pb)*0.42))

	var b strings.Builder
	b.WriteString("`FT" + primary + "Peak: " + fmtInt64(maxVal) + "`f\n")
	for row := height; row >= 1; row-- {
		rowTop := float64(row) / float64(height) * float64(maxVal)
		rowMid := (rowTop + float64(row-1)/float64(height)*float64(maxVal)) / 2
		gradTop := float64(row) / float64(height)
		gradMid := (float64(row) - 0.5) / float64(height)
		b.WriteString("│")
		for _, val := range data {
			fv := float64(val)
			upper := fv >= rowTop
			lower := fv >= rowMid || (row == 1 && fv > 0)
			switch {
			case upper:
				b.WriteString("`FT" + gradientColor(primary, secondary, gradTop, 1.3) +
					"`BT" + gradientColor(primary, secondary, gradMid, 1.3) + "▀`f`b")
			case lower:
				b.WriteString("`FT" + gradientColor(primary, secondary, gradMid, 1.3) + "▄`f")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(chartFooter(len(data), labels))
	return b.String()
}

// renderCombinedChart renders the stacked multi-series activity chart. Each
// column stacks pushes, fetches, views and downloads as proportional segments.
func renderCombinedChart(views, fetches, pushes, downloads []int64, labels []string, height int) string {
	n := len(views)
	if n == 0 || len(fetches) != n || len(pushes) != n || len(downloads) != n {
		return "No data available\n"
	}
	dim := 0.87
	cats := []struct {
		name  string
		color string
		data  []int64
	}{
		{"Pushes", gradientColor(chartColorPush, "000000", dim, 1), pushes},
		{"Fetches", gradientColor(chartColorFetch, "000000", dim, 1), fetches},
		{"Views", gradientColor(chartColorView, "000000", dim, 1), views},
		{"Downloads", gradientColor(chartColorDownload, "000000", dim, 1), downloads},
	}
	var legend []string
	for _, c := range cats {
		legend = append(legend, "`FT"+c.color+"`BT"+c.color+"██`f`b "+c.name)
	}
	var b strings.Builder
	b.WriteString(strings.Join(legend, "  ") + "\n\n")

	for row := height; row >= 1; row-- {
		lowerMin := float64(row-1) / float64(height)
		lowerMax := (float64(row) - 0.5) / float64(height)
		upperMin := lowerMax
		upperMax := float64(row) / float64(height)
		b.WriteString("│")
		for i := 0; i < n; i++ {
			var total int64
			for _, c := range cats {
				total += c.data[i]
			}
			if total == 0 {
				b.WriteString(" ")
				continue
			}
			var cum float64
			ranges := make([][2]float64, len(cats))
			for ci, c := range cats {
				start := cum / float64(total)
				cum += float64(c.data[i])
				ranges[ci] = [2]float64{start, cum / float64(total)}
			}
			upperCat, lowerCat := -1, -1
			for ci, r := range ranges {
				if upperMin < r[1] && upperMax > r[0] {
					upperCat = ci
				}
				if lowerMin < r[1] && lowerMax > r[0] {
					lowerCat = ci
				}
			}
			switch {
			case upperCat < 0 && lowerCat < 0:
				b.WriteString(" ")
			case upperCat == lowerCat:
				col := cats[upperCat].color
				b.WriteString("`FT" + col + "`BT" + col + "█`f`b")
			case upperCat >= 0 && lowerCat >= 0:
				b.WriteString("`FT" + cats[upperCat].color + "`BT" + cats[lowerCat].color + "▀`f`b")
			case upperCat >= 0:
				b.WriteString("`FT" + cats[upperCat].color + "▀`f")
			default:
				b.WriteString("`FT" + cats[lowerCat].color + "▄`f")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(chartFooter(n, labels))
	return b.String()
}

func allZero(data []int64) bool {
	for _, v := range data {
		if v != 0 {
			return false
		}
	}
	return true
}
