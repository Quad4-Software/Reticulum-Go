package site

import (
	"strings"

	"github.com/Quad4-Software/hugo-reticulum/internal/markdown"
)

// rewriteDocumentLinks scans a built Micron document for link constructs
// “ `[label`url`fields] “ and “ `[url] “ that sit outside literal
// blocks, table capture, and comment lines. Site relative URLs are resolved
// through opts.Resolver and the configured link style is applied. Links
// that already carry a scope, an external scheme, or an anchor are copied
// through unchanged.
func rewriteDocumentLinks(data []byte, opts Options, pagePath string) []byte {
	if opts.Resolver == nil {
		return data
	}
	lines := strings.Split(string(data), "\n")
	literal := false
	table := false
	for i, line := range lines {
		trimmed := strings.Trim(line, " \t\r")
		if trimmed == "`=" {
			literal = !literal
			continue
		}
		if literal {
			continue
		}
		if len(line) >= 2 && line[0] == '`' && line[1] == 't' {
			table = !table
			continue
		}
		if table || strings.HasPrefix(line, "#") {
			continue
		}
		lines[i] = rewriteLineLinks(line, opts, pagePath)
	}
	return []byte(strings.Join(lines, "\n"))
}

// rewriteLineLinks rewrites every Micron link on one line.
func rewriteLineLinks(line string, opts Options, pagePath string) string {
	var b strings.Builder
	b.Grow(len(line))
	for i := 0; i < len(line); {
		c := line[i]
		if c == '\\' && i+1 < len(line) && isMicronEscape(line[i+1]) {
			b.WriteString(line[i : i+2])
			i += 2
			continue
		}
		if c == '`' && i+1 < len(line) && line[i+1] == '[' {
			if link, next, ok := parseMicronLink(line, i+1); ok {
				b.WriteString(rebuildLink(link, opts, pagePath))
				i = next
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// micronLink is a parsed “ `[...`...`...] “ construct.
type micronLink struct {
	label    string
	url      string
	fields   string
	hasLabel bool
}

// parseMicronLink parses the link starting at the '[' index. It mirrors
// micron-parser-go's parseLink grammar: the first ']' ends the construct,
// the first backtick separates label from url, and the second separates url
// from fields.
func parseMicronLink(line string, open int) (micronLink, int, bool) {
	end := strings.IndexByte(line[open+1:], ']')
	if end < 0 {
		return micronLink{}, 0, false
	}
	end += open + 1
	data := line[open+1 : end]
	var link micronLink
	if before, after, ok := strings.Cut(data, "`"); ok {
		link.label = before
		link.hasLabel = true
		if url, fields, ok := strings.Cut(after, "`"); ok {
			if strings.IndexByte(fields, '`') >= 0 {
				return micronLink{}, 0, false
			}
			link.url = url
			link.fields = fields
		} else {
			link.url = after
		}
	} else {
		link.url = data
	}
	if link.url == "" {
		return micronLink{}, 0, false
	}
	return link, end + 1, true
}

// rebuildLink resolves the link URL and reassembles the construct with the
// configured style. URLs that do not change are emitted byte for byte.
func rebuildLink(link micronLink, opts Options, pagePath string) string {
	newURL := resolveChromeURL(link.url, link.fields, opts, pagePath)
	if newURL == link.url {
		return "`[" + link.raw() + "]"
	}
	core := "`[" + link.label + "`" + newURL
	if link.hasLabel {
		if link.fields != "" {
			core += "`" + link.fields
		}
	} else {
		core = "`[" + newURL
		if link.fields != "" {
			core += "`" + link.fields
		}
	}
	core += "]"
	return markdown.WrapLinkStyle(core, opts.LinkStyle, opts.LinkColor)
}

// raw reproduces the original inner text of the link construct.
func (l micronLink) raw() string {
	if !l.hasLabel {
		return l.url + labelFieldsSuffix(l.fields)
	}
	return l.label + "`" + l.url + labelFieldsSuffix(l.fields)
}

func labelFieldsSuffix(fields string) string {
	if fields == "" {
		return ""
	}
	return "`" + fields
}

// resolveChromeURL maps a site relative Micron URL through the resolver.
// A "|" suffix such as "|anchor=name" is preserved untouched.
func resolveChromeURL(rawURL, fields string, opts Options, pagePath string) string {
	head, suffix := rawURL, ""
	if i := strings.IndexByte(rawURL, '|'); i >= 0 {
		head, suffix = rawURL[:i], rawURL[i:]
	}
	isImage := strings.Contains(fields, "img=")
	var out string
	if isImage {
		out = opts.Resolver.Image(head, pagePath)
	} else {
		out = opts.Resolver.Link(head, pagePath)
	}
	return out + suffix
}

// isMicronEscape reports whether c can be escaped by a backslash in Micron.
func isMicronEscape(c byte) bool {
	return c == '`' || c == '\\' || c == '['
}
