package markdown

import "strings"

// wrapText wraps a Micron string to width visible columns, matching
// MarkdownToMicron._wrap_text. Link spans “ `[ ... ] “ are kept atomic so a
// wrap never splits a link across lines.
func wrapText(text string, width int) []string {
	if text == "" {
		return []string{""}
	}
	if width < 1 {
		width = 1
	}
	words := splitWrapWords(text)
	var lines []string
	var current strings.Builder
	currentWidth := 0

	flush := func() {
		if current.Len() > 0 {
			lines = append(lines, current.String())
			current.Reset()
			currentWidth = 0
		}
	}

	for _, word := range words {
		if word == "" {
			continue
		}
		wordWidth := visibleWidth(word)
		if wordWidth > width {
			flush()
			lines = append(lines, breakLongWord(word, width)...)
			continue
		}
		spaceWidth := 0
		if current.Len() > 0 {
			spaceWidth = 1
		}
		if currentWidth+spaceWidth+wordWidth <= width {
			if current.Len() > 0 {
				current.WriteByte(' ')
			}
			current.WriteString(word)
			currentWidth += spaceWidth + wordWidth
		} else {
			flush()
			current.WriteString(word)
			currentWidth = wordWidth
		}
	}
	flush()
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// splitWrapWords splits on spaces but keeps Micron link spans atomic. A link
// span runs from "`[" to the next "]".
func splitWrapWords(text string) []string {
	var words []string
	i := 0
	for i < len(text) {
		for i < len(text) && text[i] == ' ' {
			i++
		}
		if i >= len(text) {
			break
		}
		start := i
		for i < len(text) && text[i] != ' ' {
			if text[i] == '`' && i+1 < len(text) && text[i+1] == '[' {
				// Consume through the closing bracket so links with
				// spaces in the label stay atomic.
				j := strings.IndexByte(text[i:], ']')
				if j < 0 {
					i = len(text)
					break
				}
				i += j + 1
				continue
			}
			i++
		}
		words = append(words, text[start:i])
	}
	return words
}

// breakLongWord splits an overlong word into width-sized chunks, matching
// MarkdownToMicron's binary-search character break. Breaks never land inside
// a Micron link span.
func breakLongWord(word string, width int) []string {
	rs := []rune(word)
	var lines []string
	for len(rs) > 0 {
		lo, hi := 1, len(rs)
		fit := 0
		for lo <= hi {
			mid := (lo + hi) / 2
			if visibleWidth(string(rs[:mid])) <= width {
				fit = mid
				lo = mid + 1
			} else {
				hi = mid - 1
			}
		}
		if fit == 0 {
			fit = 1
		}
		for _, sp := range linkSpans(rs) {
			if fit > sp[0] && fit < sp[1] {
				if sp[0] > 0 {
					fit = sp[0]
				} else {
					fit = sp[1]
				}
				break
			}
		}
		lines = append(lines, string(rs[:fit]))
		rs = rs[fit:]
	}
	return lines
}

// linkSpans returns [start, end) rune index pairs for each "`[ ... ]" link in
// rs.
func linkSpans(rs []rune) [][2]int {
	var spans [][2]int
	for i := 0; i+1 < len(rs); i++ {
		if rs[i] != '`' || rs[i+1] != '[' {
			continue
		}
		for j := i + 2; j < len(rs); j++ {
			if rs[j] == ']' {
				spans = append(spans, [2]int{i, j + 1})
				i = j
				break
			}
		}
	}
	return spans
}
