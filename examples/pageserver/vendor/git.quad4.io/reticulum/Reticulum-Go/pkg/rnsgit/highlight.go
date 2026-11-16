// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"strings"
	"unicode"
)

// highlighter renders source code into micron markup with truecolor
// foreground spans. It is a self-contained tokenizer implementation. The
// reference defers to pygments, while this package covers the common
// language families without external dependencies.
type highlighter struct{}

func newHighlighter() *highlighter { return &highlighter{} }

// Highlight palette, a dark-theme palette in the github-dark style family.
const (
	hlKeyword  = "ff7b72"
	hlString   = "a5d6ff"
	hlNumber   = "79c0ff"
	hlComment  = "8b949e"
	hlFunc     = "d2a8ff"
	hlType     = "ffa657"
	hlOperator = "ff7b72"
	hlPreproc  = "ff7b72"
	hlName     = "e6edf3"
)

// langSpec describes the lexical shape of a language family.
type langSpec struct {
	lineComments  []string
	blockComments [][2]string
	strings       []string // quote characters that open strings
	rawStrings    []string // quote chars that skip escapes
	keywords      map[string]bool
	types         map[string]bool
	caseFold      bool
	hashComment   bool
	preprocChar   byte // '#' for c-like
	numberHex     bool
	identChars    string
}

var langSpecs = map[string]*langSpec{}

func init() {
	cLike := []string{"//"}
	cBlock := [][2]string{{"/*", "*/"}}
	langSpecs["go"] = &langSpec{
		lineComments: cLike, blockComments: cBlock, strings: []string{"\"", "`", "'"},
		rawStrings: []string{"`"}, numberHex: true,
		keywords: kwSet("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var"),
		types:    kwSet("bool byte complex64 complex128 error float32 float64 int int8 int16 int32 int64 rune string uint uint8 uint16 uint32 uint64 uintptr any comparable true false nil iota"),
	}
	langSpecs["python"] = &langSpec{
		lineComments: []string{"#"}, blockComments: [][2]string{{`"""`, `"""`}, {"'''", "'''"}},
		strings: []string{"\"", "'"}, numberHex: true,
		keywords: kwSet("and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield match case"),
		types:    kwSet("True False None self cls int str float bool list dict set tuple bytes bytearray object type"),
	}
	cspec := &langSpec{
		lineComments: cLike, blockComments: cBlock, strings: []string{"\"", "'"},
		numberHex: true, preprocChar: '#',
		keywords: kwSet("auto break case const continue default do else enum extern for goto if inline register restrict return signed sizeof static struct switch typedef union unsigned volatile while _Alignas _Atomic _Bool _Generic _Noreturn _Static_assert _Thread_local"),
		types:    kwSet("char double float int long short void size_t ssize_t uint8_t uint16_t uint32_t uint64_t int8_t int16_t int32_t int64_t bool FILE"),
	}
	langSpecs["c"] = cspec
	langSpecs["h"] = cspec
	cpp := *cspec
	cpp.keywords = kwSet("alignas alignof asm auto break case catch class concept const constexpr consteval constinit const_cast continue co_await co_return co_yield decltype default delete do dynamic_cast else enum explicit export extern false for friend goto if inline mutable namespace new noexcept nullptr operator private protected public register reinterpret_cast requires return signed sizeof static static_assert static_cast struct switch template this thread_local throw true try typedef typeid typename union unsigned using virtual volatile while")
	langSpecs["cpp"] = &cpp
	langSpecs["cxx"] = &cpp
	langSpecs["hpp"] = &cpp
	langSpecs["rust"] = &langSpec{
		lineComments: cLike, blockComments: cBlock, strings: []string{"\"", "'"},
		numberHex: true,
		keywords:  kwSet("as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while"),
		types:     kwSet("i8 i16 i32 i64 i128 isize u8 u16 u32 u64 u128 usize f32 f64 bool str char String Vec Option Result Box"),
	}
	js := &langSpec{
		lineComments: cLike, blockComments: cBlock, strings: []string{"\"", "'", "`"},
		numberHex: true,
		keywords:  kwSet("async await break case catch class const continue debugger default delete do else export extends finally for from function if import in instanceof let new of return static super switch this throw try typeof var void while with yield"),
		types:     kwSet("true false null undefined NaN Infinity number string boolean object symbol bigint Array Object Promise Map Set Date RegExp Error console window document"),
	}
	langSpecs["js"] = js
	langSpecs["javascript"] = js
	ts := *js
	ts.keywords = kwSet("abstract any as asserts async await boolean break case catch class const constructor continue debugger declare default delete do else enum export extends false finally for from function get if implements import in infer instanceof interface is keyof let module namespace never new null number object of package private protected public readonly return set static string super switch symbol this throw try type typeof undefined unique unknown var void while with yield")
	langSpecs["ts"] = &ts
	langSpecs["typescript"] = &ts
	langSpecs["java"] = &langSpec{
		lineComments: cLike, blockComments: cBlock, strings: []string{"\"", "'"},
		numberHex: true,
		keywords:  kwSet("abstract assert boolean break byte case catch char class const continue default do double else enum extends final finally float for goto if implements import instanceof int interface long native new package private protected public record return sealed short static strictfp super switch synchronized this throw throws transient try var void volatile while yield"),
		types:     kwSet("true false null String Integer Long Boolean List Map Set Optional Object System"),
	}
	langSpecs["sh"] = &langSpec{
		lineComments: []string{"#"}, strings: []string{"\"", "'", "`"},
		keywords: kwSet("if then else elif fi for while until do done case esac function in select time coproc return break continue local export readonly declare typeset set unset shift eval exec trap exit source alias unalias"),
		types:    kwSet("true false"),
	}
	langSpecs["bash"] = langSpecs["sh"]
	langSpecs["shell"] = langSpecs["sh"]
	langSpecs["env"] = langSpecs["sh"]
	langSpecs["environment"] = langSpecs["sh"]
	langSpecs["json"] = &langSpec{
		strings: []string{"\""}, numberHex: false,
		keywords: kwSet("true false null"),
	}
	langSpecs["yaml"] = &langSpec{
		lineComments: []string{"#"}, strings: []string{"\"", "'"},
		keywords: kwSet("true false yes no on off null ~"),
	}
	langSpecs["yml"] = langSpecs["yaml"]
	langSpecs["toml"] = langSpecs["yaml"]
	langSpecs["ini"] = langSpecs["yaml"]
	langSpecs["sql"] = &langSpec{
		lineComments: []string{"--"}, blockComments: cBlock, strings: []string{"'", "\""},
		caseFold: true,
		keywords: kwSet("select from where and or not null insert into values update set delete create table index view drop alter add column primary key foreign references unique check default constraint join inner left right outer full on group by order having limit offset as asc desc distinct union all exists between like in is case when then else end begin commit rollback transaction grant revoke"),
		types:    kwSet("int integer bigint smallint varchar char text date datetime timestamp boolean real numeric decimal serial blob"),
	}
	langSpecs["diff"] = &langSpec{}
}

func kwSet(words string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(words) {
		out[w] = true
	}
	return out
}

var extLang = map[string]string{
	".go": "go", ".py": "python", ".pyw": "python",
	".c": "c", ".h": "h", ".cc": "cpp", ".cpp": "cpp", ".cxx": "cpp",
	".hpp": "hpp", ".hh": "hpp",
	".rs": "rust", ".js": "js", ".mjs": "js", ".ts": "ts", ".tsx": "ts",
	".jsx": "js", ".java": "java",
	".sh": "bash", ".bash": "bash", ".zsh": "bash",
	".json": "json", ".yaml": "yaml", ".yml": "yml", ".toml": "toml",
	".ini": "ini", ".cfg": "ini", ".conf": "ini",
	".sql": "sql", ".diff": "diff", ".patch": "diff",
}

// languageFor resolves a language spec from an explicit language hint or a
// filename extension.
func languageFor(language, filename string) *langSpec {
	if language != "" {
		if s, ok := langSpecs[strings.ToLower(language)]; ok {
			return s
		}
	}
	if i := strings.LastIndexByte(filename, '.'); i >= 0 {
		if name, ok := extLang[strings.ToLower(filename[i:])]; ok {
			return langSpecs[name]
		}
	}
	return nil
}

// highlight converts source to micron markup. Unknown languages and empty
// input fall back to a plain literal block.
func (h *highlighter) highlight(content, filename, language string) string {
	if content == "" {
		return plainLiteral(content)
	}
	spec := languageFor(language, filename)
	if spec == nil {
		return plainLiteral(content)
	}
	if language == "diff" || (language == "" && (strings.HasSuffix(filename, ".diff") || strings.HasSuffix(filename, ".patch"))) {
		return highlightDiff(content)
	}
	return tokenizeToMicron(content, spec)
}

// tokenizeToMicron walks source and wraps tokens in `FT spans.
func tokenizeToMicron(src string, spec *langSpec) string {
	var b strings.Builder
	b.Grow(len(src) + len(src)/4)
	emit := func(color, text string) {
		if text == "" {
			return
		}
		b.WriteString("`FT" + color)
		emitEscaped(&b, text)
		b.WriteString("`f")
	}
	i := 0
	n := len(src)
	lineStart := true
	for i < n {
		c := src[i]
		if c == '\n' {
			b.WriteByte('\n')
			i++
			lineStart = true
			continue
		}
		// preprocessor lines for c-like
		if spec.preprocChar != 0 && lineStart && c == spec.preprocChar {
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				end = n - i
			}
			emit(hlPreproc, src[i:i+end])
			i += end
			lineStart = false
			continue
		}
		if unicode.IsSpace(rune(c)) {
			j := i
			for j < n && src[j] != '\n' && unicode.IsSpace(rune(src[j])) {
				j++
			}
			emitEscaped(&b, src[i:j])
			i = j
			continue
		}
		// comments
		matched := false
		for _, lc := range spec.lineComments {
			if strings.HasPrefix(src[i:], lc) {
				end := strings.IndexByte(src[i:], '\n')
				if end < 0 {
					end = n - i
				}
				emit(hlComment, src[i:i+end])
				i += end
				matched = true
				break
			}
		}
		if matched {
			lineStart = false
			continue
		}
		for _, bc := range spec.blockComments {
			if strings.HasPrefix(src[i:], bc[0]) {
				end := strings.Index(src[i+len(bc[0]):], bc[1])
				var stop int
				if end < 0 {
					stop = n
				} else {
					stop = i + len(bc[0]) + end + len(bc[1])
				}
				emit(hlComment, src[i:stop])
				i = stop
				matched = true
				break
			}
		}
		if matched {
			lineStart = false
			continue
		}
		// strings
		for _, q := range spec.strings {
			if len(q) == 1 && c == q[0] {
				raw := false
				for _, rq := range spec.rawStrings {
					if q == rq {
						raw = true
					}
				}
				j := i + 1
				for j < n {
					if !raw && src[j] == '\\' {
						j += 2
						continue
					}
					if src[j] == q[0] {
						j++
						break
					}
					if src[j] == '\n' && !raw {
						break
					}
					j++
				}
				emit(hlString, src[i:j])
				i = j
				matched = true
				break
			}
		}
		if matched {
			lineStart = false
			continue
		}
		// numbers
		if unicode.IsDigit(rune(c)) || (c == '.' && i+1 < n && unicode.IsDigit(rune(src[i+1]))) {
			j := i
			if spec.numberHex && c == '0' && i+1 < n && (src[i+1] == 'x' || src[i+1] == 'X') {
				j = i + 2
				for j < n && (isHexDigit(src[j]) || src[j] == '_') {
					j++
				}
			} else {
				for j < n && (unicode.IsDigit(rune(src[j])) || src[j] == '.' || src[j] == '_' ||
					src[j] == 'e' || src[j] == 'E' ||
					((src[j] == '+' || src[j] == '-') && j > i && (src[j-1] == 'e' || src[j-1] == 'E'))) {
					j++
				}
			}
			emit(hlNumber, src[i:j])
			i = j
			lineStart = false
			continue
		}
		// identifiers / keywords
		if unicode.IsLetter(rune(c)) || c == '_' {
			j := i
			for j < n && (unicode.IsLetter(rune(src[j])) || unicode.IsDigit(rune(src[j])) || src[j] == '_') {
				j++
			}
			word := src[i:j]
			lookup := word
			if spec.caseFold {
				lookup = strings.ToLower(word)
			}
			switch {
			case spec.keywords[lookup]:
				emit(hlKeyword, word)
			case spec.types[lookup]:
				emit(hlType, word)
			case j < n && src[j] == '(':
				emit(hlFunc, word)
			default:
				emitEscaped(&b, word)
			}
			i = j
			lineStart = false
			continue
		}
		// operators and punctuation
		if strings.ContainsRune("+-*/%=<>!&|^~?:", rune(c)) {
			j := i
			for j < n && strings.ContainsRune("+-*/%=<>!&|^~?:", rune(src[j])) {
				j++
			}
			emit(hlOperator, src[i:j])
			i = j
			lineStart = false
			continue
		}
		emitEscaped(&b, string(c))
		i++
		lineStart = false
	}
	return b.String()
}

// emitEscaped writes text with micron-significant characters escaped: literal
// backslashes doubled, backticks escaped, and line-leading divider/heading
// markers prefixed so they are not parsed as markup.
func emitEscaped(b *strings.Builder, text string) {
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch c {
		case '\\':
			b.WriteString("\\\\")
		case '`':
			b.WriteString("\\`")
		case '\n':
			b.WriteByte('\n')
			if i+1 < len(text) && (text[i+1] == '-' || text[i+1] == '>' || text[i+1] == '<') {
				// handled on next iteration via lineStart detection below
			}
		default:
			b.WriteByte(c)
		}
	}
}

// highlightDiff renders a unified diff with the same color scheme as the
// commit page diff formatter.
func highlightDiff(content string) string {
	return formatDiff(content)
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}
