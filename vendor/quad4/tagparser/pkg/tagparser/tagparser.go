package tagparser

import (
	"strings"

	"quad4/tagparser/internal/parser"
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
	p.parseKey()
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

func (p *tagParser) parseKey() {
	b := p.buf[:0]
	for p.Valid() {
		c := p.Read()
		switch c {
		case ',':
			p.Skip(' ')
			p.setTagOption("", string(b))
			p.buf = b
			p.parseKey()
			return
		case ':':
			key := string(b)
			p.buf = b
			p.parseValue(key)
			return
		case '\'':
			p.buf = b
			p.parseQuotedValue("")
			return
		default:
			b = append(b, c)
		}
	}

	if len(b) > 0 {
		p.setTagOption("", string(b))
	}
}

func (p *tagParser) parseValue(key string) {
	const quote = '\''
	c := p.Peek()
	if c == quote {
		p.Skip(quote)
		p.parseQuotedValue(key)
		return
	}

	b := p.buf[:0]
	for p.Valid() {
		c = p.Read()
		switch c {
		case '\\':
			b = append(b, p.Read())
		case '(':
			b = append(b, c)
			b = p.readBrackets(b)
		case ',':
			p.Skip(' ')
			p.setTagOption(key, string(b))
			p.buf = b
			p.parseKey()
			return
		default:
			b = append(b, c)
		}
	}
	p.setTagOption(key, string(b))
}

func (p *tagParser) readBrackets(b []byte) []byte {
	var lvl int
loop:
	for p.Valid() {
		c := p.Read()
		switch c {
		case '\\':
			b = append(b, p.Read())
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

func (p *tagParser) parseQuotedValue(key string) {
	const quote = '\''
	b := p.buf[:0]
	for p.Valid() {
		bb, ok := p.ReadSep(quote)
		if !ok {
			b = append(b, bb...)
			break
		}

		if len(bb) > 0 && bb[len(bb)-1] == '\\' {
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
	p.parseKey()
}
