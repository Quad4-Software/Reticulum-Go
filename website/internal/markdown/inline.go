package markdown

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

// renderInline renders the inline content of a container node (paragraph,
// heading, table cell, emphasis, link) to a Micron string.
func (r *renderer) renderInline(n ast.Node) string {
	var b strings.Builder
	r.writeChildren(&b, n)
	return b.String()
}

func (r *renderer) writeChildren(b *strings.Builder, n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.writeInline(b, c)
	}
}

func (r *renderer) writeInline(b *strings.Builder, n ast.Node) {
	switch v := n.(type) {
	case *ast.Text:
		b.WriteString(escapeText(string(v.Text(r.source))))
		if v.SoftLineBreak() {
			b.WriteByte(' ')
		}
		if v.HardLineBreak() {
			b.WriteByte('\n')
		}
	case *ast.String:
		b.WriteString(escapeText(string(v.Value)))
	case *ast.CodeSpan:
		r.writeCodeSpan(b, v)
	case *ast.Emphasis:
		cmd := "`*"
		if v.Level >= 2 {
			cmd = "`!"
		}
		b.WriteString(cmd)
		r.writeChildren(b, v)
		b.WriteString(cmd)
	case *extast.Strikethrough:
		// Micron has no strikethrough. Render the content plain.
		r.writeChildren(b, v)
	case *ast.Link:
		r.writeLink(b, v)
	case *ast.Image:
		r.writeImage(b, v)
	case *ast.AutoLink:
		r.writeAutoLink(b, v)
	case *extast.FootnoteLink:
		fmt.Fprintf(b, "[%d]", v.Index)
	case *extast.FootnoteBacklink:
		// Backlinks are navigation chrome, skip them.
	case *extast.TaskCheckBox:
		// Rendered as a [ ] / [x] marker by the list renderer.
	case *ast.RawHTML:
		b.WriteString(escapeText(string(v.Segments.Value(r.source))))
	default:
		r.writeChildren(b, n)
	}
}

func (r *renderer) writeCodeSpan(b *strings.Builder, n *ast.CodeSpan) {
	content := plainInlineText(n, r.source)
	b.WriteString("`BT383838`Fddd")
	b.WriteString(escapeText(content))
	b.WriteString("`f`b")
}

// rewrite maps a Markdown link destination through the configured rewriter.
func (r *renderer) rewrite(dest string) string {
	if r.opts.RewriteLink != nil {
		return r.opts.RewriteLink(dest)
	}
	return dest
}

// rewriteImage maps an image destination, preferring RewriteImageLink.
func (r *renderer) rewriteImage(dest string) string {
	if r.opts.RewriteImageLink != nil {
		return r.opts.RewriteImageLink(dest)
	}
	return r.rewrite(dest)
}

func (r *renderer) writeLink(b *strings.Builder, n *ast.Link) {
	label := sanitizeLabel(plainInlineText(n, r.source))
	url := sanitizeURL(r.rewrite(string(n.Destination)))
	if url == "" {
		// Degenerate link, emit the label as plain text.
		b.WriteString(escapeText(label))
		return
	}
	core := "`[" + label + "`" + url + "]"
	b.WriteString(r.wrapLinkStyle(core))
}

func (r *renderer) writeImage(b *strings.Builder, n *ast.Image) {
	alt := plainInlineText(n, r.source)
	switch r.opts.ImagePolicy {
	case ImageSkip:
		return
	case ImageAlt:
		b.WriteString(escapeText(alt))
		return
	}
	dest := string(n.Destination)
	url := sanitizeURL(r.rewriteImage(dest))
	if url == "" {
		return
	}
	label := sanitizeLabel(alt)
	if label == "" {
		label = sanitizeURL(dest)
	}
	core := "`[" + label + "`" + url + "`img=1]"
	b.WriteString(r.wrapLinkStyle(core))
}

func (r *renderer) writeAutoLink(b *strings.Builder, n *ast.AutoLink) {
	label := strings.TrimSpace(string(n.Label(r.source)))
	url := label
	if n.AutoLinkType == ast.AutoLinkEmail {
		url = "mailto:" + url
	}
	core := "`[" + sanitizeLabel(label) + "`" + sanitizeURL(url) + "]"
	b.WriteString(r.wrapLinkStyle(core))
}

// wrapLinkStyle applies the configured link decoration around a `[..`..`..]
// core. The both style matches the NomadNet defaults: underline and bold.
func (r *renderer) wrapLinkStyle(core string) string {
	return WrapLinkStyle(core, r.opts.LinkStyle, r.opts.LinkColor)
}

// WrapLinkStyle applies link decoration around a `[..`..`..] core for the
// given style and optional hex color.
func WrapLinkStyle(core string, style LinkStyle, color string) string {
	switch style {
	case LinkUnderline:
		return "`_" + core + "`_"
	case LinkBold:
		return "`!" + core + "`!"
	case LinkBoth:
		return "`_`!" + core + "`!`_"
	case LinkColor:
		if cmd, ok := colorCommand(color, false); ok {
			return cmd + core + "`f"
		}
	}
	return core
}

// colorCommand returns the Micron color command for a three or six digit hex
// value. bg selects the B variants.
func colorCommand(hex string, bg bool) (string, bool) {
	if len(hex) != 3 && len(hex) != 6 {
		return "", false
	}
	for _, c := range hex {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return "", false
		}
	}
	prefix := "`F"
	if bg {
		prefix = "`B"
	}
	if len(hex) == 6 {
		prefix += "T"
	}
	return prefix + hex, true
}

// sanitizeLabel strips characters that would break the Micron link syntax.
func sanitizeLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '`', ']', '\n', '\r':
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// sanitizeURL strips characters that would break the Micron link syntax.
func sanitizeURL(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '`', ']', '\n', '\r':
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// taskCheckbox returns the TaskCheckBox node at the start of a list item
// block, if the item begins with a GFM task list marker.
func taskCheckbox(n ast.Node) *extast.TaskCheckBox {
	if n == nil {
		return nil
	}
	if cb, ok := n.FirstChild().(*extast.TaskCheckBox); ok {
		return cb
	}
	return nil
}

// plainInlineText flattens inline content to unformatted text for use inside
// Micron link labels and image alt text.
func plainInlineText(n ast.Node, source []byte) string {
	var b strings.Builder
	var walk func(ast.Node)
	var walkChildren func(ast.Node)
	walkChildren = func(node ast.Node) {
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk = func(node ast.Node) {
		switch v := node.(type) {
		case *ast.Text:
			b.Write(v.Text(source))
			if v.SoftLineBreak() || v.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(v.Value)
		default:
			walkChildren(node)
		}
	}
	walk(n)
	return b.String()
}

// escapeText escapes the two characters that are special in Micron text mode:
// the backslash and the backtick. A backslash before any other character is
// already literal in the dialect, so doubling every backslash is safe.
func escapeText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "`", "\\`")
	return s
}

// escapeLineStart prefixes a backslash when a line begins with a character
// that would be parsed as Micron line-level syntax.
func escapeLineStart(line string) string {
	if line == "" {
		return line
	}
	switch line[0] {
	case '#', '>', '-', '<':
		return "\\" + line
	}
	return line
}
