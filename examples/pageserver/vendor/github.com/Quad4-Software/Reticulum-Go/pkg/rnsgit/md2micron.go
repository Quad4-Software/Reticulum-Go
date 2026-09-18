// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// mdToMicron converts markdown to micron markup for rendered readmes, blob
// views, release notes and work documents. It follows the conversion rules
// of the reference MarkdownToMicron: fenced code with optional highlighting,
// headers, unordered lists, quotes, tables, horizontal rules and the inline
// elements bold, italic, inline code and links. Relative links are resolved
// through a url scope so readme links can point into blob pages.
type mdToMicron struct {
	maxWidth int
	hl       *highlighter
}

func newMdToMicron(maxWidth int) *mdToMicron {
	if maxWidth <= 0 {
		maxWidth = 100
	}
	return &mdToMicron{maxWidth: maxWidth}
}

var (
	mdHeaderRe   = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	mdFenceRe    = regexp.MustCompile(`^(\s*)` + "```" + `(.*)$`)
	mdRuleRe     = regexp.MustCompile(`^(\s*)(---+|===+|\*\*\*+|___+)\s*$`)
	mdListRe     = regexp.MustCompile(`^(\s*)([-*+])\s+(.+)$`)
	mdQuoteRe    = regexp.MustCompile(`^>\s?(.*)$`)
	mdTableSepRe = regexp.MustCompile(`^\s*\|?(?:\s*:?-+:?\s*\|)+\s*$`)
	mdLinkRe     = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	mdCodeRe     = regexp.MustCompile("`([^`]+)`")
	mdBoldRe     = regexp.MustCompile(`\*\*(.+?)\*\*|__(.+?)__`)
	mdItalicRe   = regexp.MustCompile(`\*(.+?)\*|_(.+?)_`)
)

const (
	mdCodeBg       = "`BT282828"
	mdCodeBgInline = "`BT383838"
	mdCodeFg       = "`Fddd"
	mdCodeReset    = "`f`b"
	mdBullet       = "•"
	mdTableMinCol  = 3
)

// formatBlock converts a markdown document, resolving relative link targets
// through urlScope.
func (m *mdToMicron) formatBlock(text, urlScope string) string {
	if urlScope == "" {
		urlScope = ":/page/"
	}
	var out []string
	var codeBuf, quoteBuf, tableBuf []string
	inCode, inTable, inQuote := false, false, false
	codeLang := ""

	flushQuote := func() {
		if len(quoteBuf) == 0 {
			inQuote = false
			return
		}
		para := strings.Join(quoteBuf, " ")
		formatted := m.formatInline(para, urlScope)
		eff := m.maxWidth - 3
		if eff < 1 {
			eff = 1
		}
		for _, line := range wrapText(formatted, eff) {
			out = append(out, " │ "+line)
		}
		quoteBuf = nil
		inQuote = false
	}

	flushTable := func() {
		if len(tableBuf) == 0 {
			inTable = false
			return
		}
		if len(tableBuf) >= 2 && mdTableSepRe.MatchString(tableBuf[1]) {
			out = append(out, m.formatTable(tableBuf, "c", urlScope)...)
		} else {
			for _, line := range tableBuf {
				out = append(out, m.formatLine(line, urlScope))
			}
		}
		tableBuf = nil
		inTable = false
	}

	flushCode := func() {
		if len(codeBuf) == 0 {
			return
		}
		content := strings.Join(codeBuf, "\n")
		if m.hl != nil && codeLang != "" && codeLang != "rawmu" {
			out = append(out, mdCodeBg+mdCodeFg)
			out = append(out, m.hl.highlight(content, "", codeLang))
			out = append(out, mdCodeReset)
		} else if codeLang == "rawmu" {
			out = append(out, content)
		} else {
			out = append(out, mdCodeBg+mdCodeFg)
			out = append(out, "`=")
			out = append(out, escapeLiterals(content))
			out = append(out, "`=")
			out = append(out, mdCodeReset)
		}
		codeBuf = nil
	}

	for _, line := range strings.Split(text, "\n") {
		if fm := mdFenceRe.FindStringSubmatch(line); fm != nil {
			flushQuote()
			flushTable()
			if !inCode {
				inCode = true
				codeLang = strings.TrimSpace(fm[2])
			} else {
				flushCode()
				inCode = false
				codeLang = ""
			}
			continue
		}
		if inCode {
			codeBuf = append(codeBuf, line)
			continue
		}
		if qm := mdQuoteRe.FindStringSubmatch(line); qm != nil {
			if !inQuote {
				flushTable()
				inQuote = true
			}
			quoteBuf = append(quoteBuf, qm[1])
			continue
		}
		if inQuote {
			flushQuote()
			if strings.TrimSpace(line) == "" {
				out = append(out, "")
				continue
			}
		}
		if isTableRow(line) {
			if !inTable {
				inTable = true
			}
			tableBuf = append(tableBuf, line)
			continue
		}
		if inTable {
			flushTable()
		}
		out = append(out, m.formatLine(line, urlScope))
	}
	if inQuote {
		flushQuote()
	}
	if inTable {
		flushTable()
	}
	if inCode {
		flushCode()
	}
	return strings.Join(out, "\n")
}

// formatLine converts one markdown line.
func (m *mdToMicron) formatLine(line, urlScope string) string {
	line = strings.ReplaceAll(line, "\\", "\\\\")
	if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") && !strings.HasPrefix(line, "- ") {
		line = "\\" + line
	}
	if strings.HasPrefix(line, "<") {
		line = "\\" + line
	}
	if mdRuleRe.MatchString(line) {
		return "-"
	}
	if hm := mdHeaderRe.FindStringSubmatch(line); hm != nil {
		level := len(hm[1])
		if level > 6 {
			level = 6
		}
		return strings.Repeat(">", level) + m.formatInline(hm[2], urlScope)
	}
	if lm := mdListRe.FindStringSubmatch(line); lm != nil {
		return lm[1] + " " + mdBullet + " " + m.formatInline(lm[3], urlScope)
	}
	return m.formatInline(line, urlScope)
}

// formatInline converts inline markdown: links, inline code, bold, italic.
// Links without a :/ scheme resolve through the url scope.
func (m *mdToMicron) formatInline(text, urlScope string) string {
	var links [][2]string
	var codes []string

	text = mdLinkRe.ReplaceAllStringFunc(text, func(s string) string {
		parts := mdLinkRe.FindStringSubmatch(s)
		links = append(links, [2]string{parts[1], parts[2]})
		return fmt.Sprintf("\x00LINK%d\x00", len(links)-1)
	})
	text = mdCodeRe.ReplaceAllStringFunc(text, func(s string) string {
		parts := mdCodeRe.FindStringSubmatch(s)
		codes = append(codes, parts[1])
		return fmt.Sprintf("\x00CODE%d\x00", len(codes)-1)
	})
	// Escape remaining stray backticks so literal text cannot open Micron
	// format spans. Placeholders contain no backticks, so they survive.
	text = escapeLiterals(text)
	text = mdBoldRe.ReplaceAllStringFunc(text, func(s string) string {
		parts := mdBoldRe.FindStringSubmatch(s)
		content := parts[1]
		if content == "" {
			content = parts[2]
		}
		return "`!" + content + "`!"
	})
	text = mdItalicRe.ReplaceAllStringFunc(text, func(s string) string {
		parts := mdItalicRe.FindStringSubmatch(s)
		content := parts[1]
		if content == "" {
			content = parts[2]
		}
		return "`*" + content + "`*"
	})

	linkRestore := regexp.MustCompile(`\x00LINK(\d+)\x00`)
	text = linkRestore.ReplaceAllStringFunc(text, func(s string) string {
		idx, _ := strconv.Atoi(linkRestore.FindStringSubmatch(s)[1])
		label, link := links[idx][0], links[idx][1]
		anchor := ""
		if i := strings.IndexByte(link, '#'); i >= 0 {
			anchor = link[i+1:]
			link = link[:i]
		}
		if !strings.Contains(link, ":/") {
			link = urlScope + link
			if anchor != "" {
				link += "|anchor=" + anchor
			}
		}
		label = strings.ReplaceAll(label, "`", "")
		return "`_`!`[" + label + "`" + link + "]`!`_"
	})

	codeRestore := regexp.MustCompile(`\x00CODE(\d+)\x00`)
	text = codeRestore.ReplaceAllStringFunc(text, func(s string) string {
		idx, _ := strconv.Atoi(codeRestore.FindStringSubmatch(s)[1])
		return mdCodeBgInline + mdCodeFg + escapeLiterals(codes[idx]) + mdCodeReset
	})
	return text
}

func escapeLiterals(s string) string {
	return strings.ReplaceAll(s, "`", "\\`")
}

// isTableRow reports whether a line can start or continue a table.
func isTableRow(line string) bool {
	if !strings.Contains(line, "|") {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(line), "|") ||
		strings.Contains(strings.Trim(strings.TrimSpace(line), "|"), "|")
}

func parseTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	var cells []string
	var cur strings.Builder
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '|':
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	cells = append(cells, strings.TrimSpace(cur.String()))
	return cells
}

func parseTableAlignments(line string) []string {
	var out []string
	for _, cell := range parseTableRow(line) {
		cell = strings.TrimSpace(cell)
		switch {
		case strings.HasPrefix(cell, ":") && strings.HasSuffix(cell, ":"):
			out = append(out, "center")
		case strings.HasSuffix(cell, ":"):
			out = append(out, "right")
		default:
			out = append(out, "left")
		}
	}
	return out
}

// visibleWidth measures text without micron tags.
func visibleWidth(text string) int {
	return len(stripMicron(text))
}

var micronTagRes = []*regexp.Regexp{
	regexp.MustCompile("`[FB]T[0-9a-fA-F]{6}"),
	regexp.MustCompile("`[FB][0-9a-fA-F]{3}"),
	regexp.MustCompile("`[!*_=]"),
	regexp.MustCompile("`f`b"),
	regexp.MustCompile("`f"),
	regexp.MustCompile("`b"),
}

func stripMicron(text string) string {
	for _, re := range micronTagRes {
		text = re.ReplaceAllString(text, "")
	}
	return text
}

// padCell pads or truncates a cell to width with alignment.
func padCell(text string, width int, align string) string {
	text = truncateCell(text, width)
	padding := width - visibleWidth(text)
	if padding < 0 {
		padding = 0
	}
	switch align {
	case "right":
		return strings.Repeat(" ", padding) + text
	case "center":
		left := padding / 2
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", padding-left)
	default:
		return text + strings.Repeat(" ", padding)
	}
}

// truncateCell cuts a cell at width and closes any micron tags left open.
func truncateCell(text string, width int) string {
	if visibleWidth(text) <= width {
		return text
	}
	point := len(text)
	for point > 0 && visibleWidth(text[:point]) >= width {
		point--
	}
	truncated := text[:point]
	active := map[byte]bool{}
	fgActive, bgActive := false, false
	for i := 0; i < len(truncated); {
		if truncated[i] != '`' || i+1 >= len(truncated) {
			i++
			continue
		}
		tag := truncated[i+1]
		switch {
		case tag == '!' || tag == '*' || tag == '_' || tag == '=':
			active[tag] = !active[tag]
			i += 2
		case tag == 'f':
			fgActive = false
			i += 2
		case tag == 'b':
			bgActive = false
			i += 2
		case tag == 'F':
			fgActive = true
			if i+2 < len(truncated) && truncated[i+2] == 'T' {
				i += 8
			} else {
				i += 5
			}
		case tag == 'B':
			bgActive = true
			if i+2 < len(truncated) && truncated[i+2] == 'T' {
				i += 8
			} else {
				i += 5
			}
		default:
			i++
		}
	}
	var closers strings.Builder
	if fgActive {
		closers.WriteString("`f")
	}
	if bgActive {
		closers.WriteString("`b")
	}
	for tag, on := range active {
		if on {
			closers.WriteString("`" + string(tag))
		}
	}
	return truncated + closers.String() + "…"
}

// formatTable renders a markdown table with box-drawing borders, shrinking
// the widest columns to fit maxWidth.
func (m *mdToMicron) formatTable(rows []string, align, urlScope string) []string {
	if len(rows) < 2 {
		return rows
	}
	header := parseTableRow(rows[0])
	alignments := parseTableAlignments(rows[1])
	for len(alignments) < len(header) {
		alignments = append(alignments, "left")
	}
	alignments = alignments[:len(header)]
	var data [][]string
	for _, row := range rows[2:] {
		cells := parseTableRow(row)
		for len(cells) < len(header) {
			cells = append(cells, "")
		}
		data = append(data, cells[:len(header)])
	}

	widths := make([]int, len(header))
	for _, row := range append([][]string{header}, data...) {
		for i, cell := range row {
			if w := visibleWidth(m.formatInline(cell, urlScope)); w > widths[i] {
				widths[i] = w
			}
		}
	}
	for i := range widths {
		if widths[i] < mdTableMinCol {
			widths[i] = mdTableMinCol
		}
	}
	total := 1
	for _, w := range widths {
		total += w + 3
	}
	if total > m.maxWidth {
		excess := total - m.maxWidth
		order := make([]int, len(widths))
		for i := range order {
			order[i] = i
		}
		sortByWidth(order, widths)
		for _, i := range order {
			if excess <= 0 {
				break
			}
			red := excess
			if w := widths[i] - mdTableMinCol; w < red {
				red = w
			}
			if red > 0 {
				widths[i] -= red
				excess -= red
			}
		}
	}

	border := func(left, mid, right string) string {
		var b strings.Builder
		b.WriteString(left)
		for i, w := range widths {
			b.WriteString(strings.Repeat("─", w+2))
			if i < len(widths)-1 {
				b.WriteString(mid)
			} else {
				b.WriteString(right)
			}
		}
		return b.String()
	}
	rowLine := func(cells []string, aligns []string) string {
		var b strings.Builder
		b.WriteString("│")
		for i, cell := range cells {
			b.WriteString(" " + padCell(m.formatInline(cell, urlScope), widths[i], aligns[i]) + " │")
		}
		return b.String()
	}

	out := []string{"`" + align}
	out = append(out, escapeLiterals(border("┌", "┬", "┐")))
	out = append(out, escapeLiterals(rowLine(header, repeat("left", len(header)))))
	out = append(out, escapeLiterals(border("├", "┼", "┤")))
	for _, row := range data {
		out = append(out, rowLine(row, alignments))
	}
	out = append(out, escapeLiterals(border("└", "┴", "┘")))
	out = append(out, "`a")
	return out
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func sortByWidth(order []int, widths []int) {
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && widths[order[j-1]] < widths[order[j]]; j-- {
			order[j-1], order[j] = order[j], order[j-1]
		}
	}
}

// wrapText wraps text at a visible width, force-breaking overlong words.
func wrapText(text string, width int) []string {
	if text == "" {
		return []string{""}
	}
	if width < 1 {
		width = 1
	}
	var lines []string
	var cur strings.Builder
	curWidth := 0
	for _, word := range strings.Fields(text) {
		ww := visibleWidth(word)
		if ww > width {
			if cur.Len() > 0 {
				lines = append(lines, cur.String())
				cur.Reset()
				curWidth = 0
			}
			for len(word) > 0 {
				fit := 1
				for len(word) > fit && visibleWidth(word[:fit+1]) <= width {
					fit++
				}
				lines = append(lines, word[:fit])
				word = word[fit:]
			}
			continue
		}
		space := 0
		if cur.Len() > 0 {
			space = 1
		}
		if curWidth+space+ww <= width {
			if space > 0 {
				cur.WriteString(" ")
			}
			cur.WriteString(word)
			curWidth += space + ww
		} else {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(word)
			curWidth = ww
		}
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}
