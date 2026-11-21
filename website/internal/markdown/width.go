package markdown

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	visStrip1 = regexp.MustCompile("`[FB][0-9a-fA-F]{3}")
	visStrip2 = regexp.MustCompile("`[FB]T[0-9a-fA-F]{6}")
	visStrip3 = regexp.MustCompile("`[!*_=]")
	visStrip4 = regexp.MustCompile("`f`b")
	visStrip5 = regexp.MustCompile("`f")
	visStrip6 = regexp.MustCompile("`b")
)

// visibleWidth returns the display width of a Micron string with formatting
// commands removed, matching MarkdownToMicron._visible_width.
func visibleWidth(s string) int {
	s = visStrip1.ReplaceAllString(s, "")
	s = visStrip2.ReplaceAllString(s, "")
	s = visStrip3.ReplaceAllString(s, "")
	s = visStrip4.ReplaceAllString(s, "")
	s = visStrip5.ReplaceAllString(s, "")
	s = visStrip6.ReplaceAllString(s, "")
	return utf8.RuneCountInString(s)
}

// padCell pads a Micron cell string to width visible columns.
func padCell(text string, width int, align string) string {
	text = truncateCell(text, width)
	padding := width - visibleWidth(text)
	if padding <= 0 {
		return text
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

// truncateCell shortens a cell to width visible columns, closing any open
// Micron formatting commands before the ellipsis. Ported from
// MarkdownToMicron._truncate_cell.
func truncateCell(text string, width int) string {
	if visibleWidth(text) <= width {
		return text
	}
	rs := []rune(text)
	// Find the longest prefix whose visible width stays under width.
	lo, hi := 0, len(rs)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if visibleWidth(string(rs[:mid])) < width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	truncated := string(rs[:lo])

	active := map[byte]bool{}
	fgActive, bgActive := false, false
	for i := 0; i < len(truncated); i++ {
		if truncated[i] != '`' || i+1 >= len(truncated) {
			continue
		}
		tag := truncated[i+1]
		switch {
		case strings.IndexByte("!*_=", tag) >= 0:
			if active[tag] {
				delete(active, tag)
			} else {
				active[tag] = true
			}
			i++
		case tag == 'f':
			fgActive = false
			i++
		case tag == 'b':
			bgActive = false
			i++
		case tag == 'F':
			fgActive = true
			if i+2 < len(truncated) && truncated[i+2] == 'T' {
				i += 7
			} else {
				i += 4
			}
		case tag == 'B':
			bgActive = true
			if i+2 < len(truncated) && truncated[i+2] == 'T' {
				i += 7
			} else {
				i += 4
			}
		}
	}

	var closers strings.Builder
	if fgActive {
		closers.WriteString("`f")
	}
	if bgActive {
		closers.WriteString("`b")
	}
	for _, tag := range []byte{'!', '*', '_', '='} {
		if active[tag] {
			closers.WriteByte('`')
			closers.WriteByte(tag)
		}
	}
	return truncated + closers.String() + "…"
}
