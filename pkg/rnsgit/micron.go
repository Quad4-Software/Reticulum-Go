// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Micron markup primitives used by the rngit page server. These emit the same
// markup the Python nomadnetwork node emits, built up from small helpers
// rather than format-string templates.

const (
	micronFgReset = "`f"
	micronBgReset = "`b"
)

// mEscape escapes text so micron does not interpret markup characters in it.
func mEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "`", "\\`")
}

// mEscapeLabel strips link markup characters from a link label.
func mEscapeLabel(s string) string {
	s = strings.ReplaceAll(s, "[", "")
	s = strings.ReplaceAll(s, "]", "")
	return strings.ReplaceAll(s, "`", "")
}

// mFieldValue encodes a link field value like NomadNet clients expect.
func mFieldValue(v string) string {
	return url.QueryEscape(v)
}

// mLink builds a plain micron link: `label`:path`k=v|k=v]
func mLink(label, pagePath string, fields map[string]string) string {
	var b strings.Builder
	b.WriteString("`[")
	b.WriteString(mEscapeLabel(label))
	b.WriteString("`:")
	b.WriteString(pagePath)
	if len(fields) > 0 {
		b.WriteString(mFields(fields))
	}
	b.WriteString("]")
	return b.String()
}

// mLinkR builds a regular-strength link, matching the Python m_link_r helper.
func mLinkR(label, pagePath string, fields map[string]string) string {
	return mLink(label, pagePath, fields)
}

// mLinkStrong builds a bold link, matching the Python m_link helper which
// wraps the link in bold markers.
func mLinkStrong(label, pagePath string, fields map[string]string) string {
	return "`!" + mLink(label, pagePath, fields) + "`!"
}

// mLinkE builds an external cross-node link carrying a remote destination
// hash, matching the Python m_link_e helper.
func mLinkE(label, remoteDestHash, pagePath string, fields map[string]string) string {
	var b strings.Builder
	b.WriteString("`[")
	b.WriteString(mEscapeLabel(label))
	b.WriteString("`")
	b.WriteString(remoteDestHash)
	b.WriteString(":")
	b.WriteString(pagePath)
	if len(fields) > 0 {
		b.WriteString(mFields(fields))
	}
	b.WriteString("]")
	return b.String()
}

func mFields(fields map[string]string) string {
	var b strings.Builder
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("`")
	for i, k := range keys {
		if i > 0 {
			b.WriteString("|")
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(mFieldValue(fields[k]))
	}
	return b.String()
}

// mHeading returns the micron heading marker for level (1-6).
func mHeading(level int) string {
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}
	return strings.Repeat(">", level)
}

func mDim(s string) string { return "`F666" + s + "`f" }

// mFg wraps text in a 3- or 6-digit micron foreground color.
func mFg(s, hexcolor string) string {
	return "`F" + hexcolor + s + micronFgReset
}

// mDivider returns a horizontal divider.
func mDivider() string { return "-" }

// Page icons, matching the glyph roles in the Python rngit page server. The
// nerd font set is used unless unicode_icons is configured.
type pageIcons struct {
	Sep     string
	Folder  string
	File    string
	Branch  string
	Tag     string
	Commits string
	Stats   string
	Heart   string
	Package string
	Work    string
}

var nerdIcons = pageIcons{
	Sep: "•", Folder: "\U000f0256", File: "\U000f0216",
	Branch: "\U000f062c", Tag: "\U000f04fc", Commits: "\U000f02da",
	Stats: "\U000f0201", Heart: "\U000f02d1", Package: "\U000f03d7",
	Work: "\U000f1323",
}

var unicodeIcons = pageIcons{
	Sep: "•", Folder: "\U0001f5c0", File: "\U0001f5ce",
	Branch: "⑃", Tag: "⌆", Commits: "\U0001f5b9",
	Stats: "\U0001f5e0", Heart: "♥", Package: "◇",
	Work: "☸",
}

// icons returns the configured icon set.
func (n *Node) icons() pageIcons {
	if n.cfg.UnicodeIcons {
		return unicodeIcons
	}
	return nerdIcons
}

// prettySize renders a byte count in binary units, matching RNS.prettysize.
func prettySize(num float64) string {
	suffix := "B"
	if num >= 1024 {
		num /= 1024
		suffix = "KB"
	}
	if num >= 1024 {
		num /= 1024
		suffix = "MB"
	}
	if num >= 1024 {
		num /= 1024
		suffix = "GB"
	}
	if num >= 1024 {
		num /= 1024
		suffix = "TB"
	}
	if num >= 1024 {
		num /= 1024
		suffix = "PB"
	}
	if suffix == "B" {
		return fmt.Sprintf("%.0f %s", num, suffix)
	}
	return fmt.Sprintf("%.2f %s", num, suffix)
}

// prettyTime renders a duration like RNS.prettytime: compact unit suffixes.
func prettyTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	d := time.Duration(seconds * float64(time.Second))
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d weeks", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%d months", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%d years", int(d.Hours()/(24*365)))
	}
}

// prettyTimeAgo renders "x ago" for a unix timestamp.
func prettyTimeAgo(ts int64) string {
	if ts <= 0 {
		return "never"
	}
	ago := time.Since(time.Unix(ts, 0)).Seconds()
	if ago < 0 {
		ago = 0
	}
	return prettyTime(ago) + " ago"
}

// prettyDate renders an absolute timestamp like the Python pages show.
func prettyDate(ts int64) string {
	return time.Unix(ts, 0).UTC().Format("2006-01-02 15:04:05 UTC")
}
