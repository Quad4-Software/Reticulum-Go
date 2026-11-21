package tagparser

import (
	"strings"

	"github.com/Quad4-Software/tagparser/v2/internal/parser"
)

type Tag struct {
	Name    string
	Options map[string]string
}

func (t *Tag) HasOption(name string) bool {
	if t.Options == nil {
		return false
	}
	_, ok := t.Options[name]
	return ok
}

func Parse(s string) *Tag {
	p := &tagParser{Parser: parser.NewString(s)}
	p.parse()
	return &p.Tag
}

type tagParser struct {
	*parser.Parser

	Tag Tag
	// buf is reused across segments; nil until the first append. Each segment converts to a
	// fresh string via string(b) before another segment overwrites the backing array.
	buf     []byte
	hasName bool
}

func (p *tagParser) setTagOption(key, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	if !p.hasName {
		p.hasName = true
		if key == "" {
			p.Tag.Name = value
			return
		}
	}
	if p.Tag.Options == nil {
		p.Tag.Options = make(map[string]string)
	}
	if key == "" {
		p.Tag.Options[value] = ""
	} else {
		p.Tag.Options[key] = value
	}
}

// parse consumes one comma separated segment per iteration. The segment
// scanners below report through their return value whether another segment
// follows, so the input length cannot grow the call stack.
func (p *tagParser) parse() {
	for p.parseKey() {
	}
}

func (p *tagParser) parseKey() bool {
	b := p.buf[:0]
	for p.Valid() {
		c := p.Read()
		switch c {
		case ',':
			p.Skip(' ')
			p.setTagOption("", string(b))
			p.buf = b
			return true
		case ':':
			key := string(b)
			p.buf = b
			return p.parseValue(key)
		case '\'':
			p.buf = b
			return p.parseQuotedValue("")
		default:
			b = append(b, c)
		}
	}

	if len(b) > 0 {
		p.setTagOption("", string(b))
	}
	return false
}

func (p *tagParser) parseValue(key string) bool {
	const quote = '\''
	c := p.Peek()
	if c == quote {
		p.Skip(quote)
		return p.parseQuotedValue(key)
	}

	b := p.buf[:0]
	for p.Valid() {
		c = p.Read()
		switch c {
		case '\\':
			if p.Valid() {
				b = append(b, p.Read())
			} else {
				b = append(b, c)
			}
		case '(':
			b = append(b, c)
			b = p.readBrackets(b)
		case ',':
			p.Skip(' ')
			p.setTagOption(key, string(b))
			p.buf = b
			return true
		default:
			b = append(b, c)
		}
	}
	p.setTagOption(key, string(b))
	return false
}

func (p *tagParser) readBrackets(b []byte) []byte {
	var lvl int
loop:
	for p.Valid() {
		c := p.Read()
		switch c {
		case '\\':
			if p.Valid() {
				b = append(b, p.Read())
			} else {
				b = append(b, c)
			}
		case '(':
			b = append(b, c)
			lvl++
		case ')':
			b = append(b, c)
			lvl--
			if lvl < 0 {
				break loop
			}
		default:
			b = append(b, c)
		}
	}
	return b
}

func (p *tagParser) parseQuotedValue(key string) bool {
	const quote = '\''
	b := p.buf[:0]
	for p.Valid() {
		bb, ok := p.ReadSep(quote)
		if !ok {
			b = append(b, bb...)
			break
		}

		// A quote preceded by an odd number of backslashes is escaped; the
		// last backslash is dropped and the quote kept literally. An even
		// count (including a double backslash) leaves the quote as the
		// terminator.
		if n := trailingBackslashes(bb); n%2 == 1 {
			b = append(b, bb[:len(bb)-1]...)
			b = append(b, quote)
			continue
		}

		b = append(b, bb...)
		break
	}

	p.setTagOption(key, string(b))
	p.buf = b
	if p.Skip(',') {
		p.Skip(' ')
	}
	return true
}

func trailingBackslashes(b []byte) int {
	n := 0
	for i := len(b) - 1; i >= 0 && b[i] == '\\'; i-- {
		n++
	}
	return n
}
