package markdown

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

var parserInstance = goldmark.New(
	goldmark.WithExtensions(extension.GFM, extension.Footnote),
)

// Convert renders Markdown source as Micron markup. The result always ends
// with a single newline.
func Convert(src string, opts Options) (string, error) {
	source := []byte(src)
	doc := parserInstance.Parser().Parse(text.NewReader(source))
	r := &renderer{opts: opts, maxWidth: opts.MaxWidth, source: source}
	if r.maxWidth <= 0 {
		r.maxWidth = DefaultMaxWidth
	}
	r.renderDocument(doc)
	return strings.Join(r.lines, "\n") + "\n", nil
}

type renderer struct {
	opts      Options
	maxWidth  int
	wrapWidth int
	source    []byte
	lines     []string
}

// child returns a renderer that shares configuration and source for nested
// contexts such as blockquotes and list items.
func (r *renderer) child() *renderer {
	return &renderer{opts: r.opts, maxWidth: r.maxWidth, source: r.source}
}

func (r *renderer) renderDocument(n ast.Node) {
	r.renderSpaced(n)
}

// renderSpaced renders child blocks with a blank line between them.
func (r *renderer) renderSpaced(n ast.Node) {
	first := true
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if !first {
			r.lines = append(r.lines, "")
		}
		first = false
		r.renderBlock(c)
	}
}

func (r *renderer) renderBlock(n ast.Node) {
	switch v := n.(type) {
	case *ast.Heading:
		r.renderHeading(v)
	case *ast.Paragraph, *ast.TextBlock:
		r.emitInlineLines(r.renderInline(v))
	case *ast.CodeBlock:
		r.renderCodeBlock(string(v.Lines().Value(r.source)))
	case *ast.FencedCodeBlock:
		r.renderFencedCode(v)
	case *ast.Blockquote:
		r.renderQuote(v)
	case *ast.List:
		r.renderList(v, "")
	case *ast.ThematicBreak:
		r.lines = append(r.lines, "-")
	case *extast.Table:
		r.renderTable(v)
	case *extast.FootnoteList:
		r.renderFootnoteList(v)
	case *ast.HTMLBlock:
		r.renderHTMLBlock(v)
	default:
		if n.FirstChild() != nil {
			r.renderBlocks(n)
		}
	}
}

func (r *renderer) renderBlocks(n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.renderBlock(c)
	}
}

func (r *renderer) renderHeading(n *ast.Heading) {
	depth := n.Level
	if depth > 16 {
		depth = 16
	}
	prefix := strings.Repeat(">", depth)
	// Headings are single lines in Micron, so hard breaks become spaces.
	text := strings.ReplaceAll(r.renderInline(n), "\n", " ")
	r.lines = append(r.lines, prefix+" "+text)
}

// emitInlineLines splits rendered inline text on newlines (hard breaks) and
// applies line-start escaping to each resulting line. When wrapWidth is set,
// lines are wrapped to that visible width first.
func (r *renderer) emitInlineLines(s string) {
	for _, line := range strings.Split(s, "\n") {
		if r.wrapWidth > 0 {
			for _, wl := range wrapText(line, r.wrapWidth) {
				r.lines = append(r.lines, escapeLineStart(wl))
			}
		} else {
			r.lines = append(r.lines, escapeLineStart(line))
		}
	}
}

func (r *renderer) renderQuote(n *ast.Blockquote) {
	sub := r.child()
	// Quote text wraps at max width minus the " │ " prefix, matching
	// MarkdownToMicron. Nested quotes shrink the budget further.
	base := r.wrapWidth
	if base <= 0 {
		base = r.maxWidth
	}
	sub.wrapWidth = max(base-3, 1)
	sub.renderSpaced(n)
	for _, line := range sub.lines {
		if line == "" {
			r.lines = append(r.lines, "")
		} else {
			r.lines = append(r.lines, " │ "+line)
		}
	}
}

func (r *renderer) renderList(n *ast.List, indent string) {
	index := 0
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		li, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}
		index++
		marker := "• "
		if n.IsOrdered() {
			start := n.Start
			if start == 0 {
				start = 1
			}
			marker = fmt.Sprintf("%d. ", start+index-1)
		}
		if cb := taskCheckbox(firstItemBlock(li)); cb != nil {
			if cb.IsChecked {
				marker += "[x] "
			} else {
				marker += "[ ] "
			}
		}
		cont := indent + strings.Repeat(" ", len([]rune(marker)))
		first := true
		for c := li.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *ast.Paragraph, *ast.TextBlock:
				for i, line := range strings.Split(r.renderInline(v), "\n") {
					if line == "" {
						r.lines = append(r.lines, "")
						continue
					}
					if first && i == 0 {
						r.lines = append(r.lines, indent+marker+escapeLineStart(line))
					} else {
						r.lines = append(r.lines, cont+escapeLineStart(line))
					}
				}
				first = false
			case *ast.List:
				r.renderList(v, cont)
				first = false
			default:
				sub := r.child()
				sub.renderBlock(c)
				for _, line := range sub.lines {
					if line == "" {
						r.lines = append(r.lines, "")
						continue
					}
					if first {
						r.lines = append(r.lines, indent+marker+line)
						first = false
					} else {
						r.lines = append(r.lines, cont+line)
					}
				}
			}
		}
		if first {
			r.lines = append(r.lines, indent+marker)
		}
	}
}

// firstItemBlock returns the first paragraph-like block of a list item.
// Tight lists use TextBlock, loose lists use Paragraph.
func firstItemBlock(li *ast.ListItem) ast.Node {
	for c := li.FirstChild(); c != nil; c = c.NextSibling() {
		switch c.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			return c
		case *ast.List:
			return nil
		}
	}
	return nil
}

func (r *renderer) renderCodeBlock(text string) {
	r.lines = append(r.lines, "`BT282828`Fddd", "`=")
	for _, line := range splitLines(text) {
		r.lines = append(r.lines, literalLine(line))
	}
	r.lines = append(r.lines, "`=", "`f`b")
}

func (r *renderer) renderFencedCode(n *ast.FencedCodeBlock) {
	body := string(n.Lines().Value(r.source))
	lang := strings.ToLower(strings.TrimSpace(string(n.Language(r.source))))
	if lang == "rawmu" {
		// rawmu fences pass Micron source through untouched.
		for _, line := range splitLines(body) {
			r.lines = append(r.lines, line)
		}
		return
	}
	r.renderCodeBlock(body)
}

// literalLine escapes a line for inclusion in a `= literal block. A line that
// would toggle the block is rewritten in the dialect's escaped form.
func literalLine(line string) string {
	if strings.Trim(line, " \t\r") == "`=" {
		return "\\`="
	}
	return line
}

func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func (r *renderer) renderHTMLBlock(n *ast.HTMLBlock) {
	for _, line := range splitLines(string(n.Lines().Value(r.source))) {
		r.lines = append(r.lines, escapeLineStart(escapeText(line)))
	}
}

func (r *renderer) renderFootnoteList(n *extast.FootnoteList) {
	index := 0
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		fn, ok := c.(*extast.Footnote)
		if !ok {
			continue
		}
		index++
		sub := r.child()
		sub.renderSpaced(fn)
		firstLine := true
		for _, line := range sub.lines {
			if line == "" {
				r.lines = append(r.lines, "")
				continue
			}
			if firstLine {
				r.lines = append(r.lines, fmt.Sprintf("%d. %s", index, line))
				firstLine = false
			} else {
				r.lines = append(r.lines, "   "+line)
			}
		}
	}
}
