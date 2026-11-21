package markdown

import (
	"strings"
	"testing"
)

func convert(t *testing.T, src string, opts Options) string {
	t.Helper()
	out, err := Convert(src, opts)
	if err != nil {
		t.Fatalf("Convert(%q): %v", src, err)
	}
	return out
}

func TestConvertBlocks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "heading levels",
			in:   "# One\n\n### Three\n",
			want: "> One\n\n>>> Three\n",
		},
		{
			name: "heading level six",
			in:   "###### deep\n",
			want: ">>>>>> deep\n",
		},
		{
			name: "paragraph",
			in:   "hello world\n",
			want: "hello world\n",
		},
		{
			name: "paragraph hard break",
			in:   "line one  \nline two\n",
			want: "line one\nline two\n",
		},
		{
			name: "thematic break",
			in:   "above\n\n---\n\nbelow\n",
			want: "above\n\n-\n\nbelow\n",
		},
		{
			name: "unordered list",
			in:   "- a\n- b\n",
			want: "• a\n• b\n",
		},
		{
			name: "ordered list",
			in:   "1. a\n2. b\n",
			want: "1. a\n2. b\n",
		},
		{
			name: "ordered list start offset",
			in:   "3. a\n4. b\n",
			want: "3. a\n4. b\n",
		},
		{
			name: "nested list",
			in:   "- a\n  - b\n- c\n",
			want: "• a\n  • b\n• c\n",
		},
		{
			name: "task list",
			in:   "- [ ] todo\n- [x] done\n",
			want: "• [ ] todo\n• [x] done\n",
		},
		{
			name: "blockquote",
			in:   "> quoted text\n",
			want: " │ quoted text\n",
		},
		{
			name: "blockquote multiline",
			in:   "> one\n>\n> two\n",
			want: " │ one\n\n │ two\n",
		},
		{
			name: "fenced code",
			in:   "```go\nfmt.Println(1)\n```\n",
			want: "`BT282828`Fddd\n`=\nfmt.Println(1)\n`=\n`f`b\n",
		},
		{
			name: "indented code",
			in:   "para\n\n    code line\n",
			want: "para\n\n`BT282828`Fddd\n`=\ncode line\n`=\n`f`b\n",
		},
		{
			name: "rawmu fence passthrough",
			in:   "```rawmu\n> already micron\n```\n",
			want: "> already micron\n",
		},
		{
			name: "html block escaped",
			in:   "<div>hi</div>\n",
			want: "\\<div>hi</div>\n",
		},
		{
			name: "footnote",
			in:   "text[^1]\n\n[^1]: the note\n",
			want: "text[1]\n\n1. the note\n",
		},
		{
			name: "definition list falls back",
			in:   "Term\n: def\n",
			want: "Term : def\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := convert(t, tc.in, Options{})
			if got != tc.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

func TestConvertInline(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		opts Options
	}{
		{
			name: "bold",
			in:   "**bold**\n",
			want: "`!bold`!\n",
		},
		{
			name: "italic star",
			in:   "*it*\n",
			want: "`*it`*\n",
		},
		{
			name: "italic underscore",
			in:   "_it_\n",
			want: "`*it`*\n",
		},
		{
			name: "bold italic",
			in:   "***both***\n",
			want: "`*`!both`!`*\n",
		},
		{
			name: "strikethrough plain",
			in:   "~~gone~~\n",
			want: "gone\n",
		},
		{
			name: "inline code",
			in:   "use `x++` here\n",
			want: "use `BT383838`Fddd" + "x++" + "`f`b here\n",
		},
		{
			name: "link plain",
			in:   "[text](https://example.com)\n",
			want: "`[text`https://example.com]\n",
		},
		{
			name: "link underline",
			in:   "[text](https://example.com)\n",
			want: "`_`[text`https://example.com]`_\n",
			opts: Options{LinkStyle: LinkUnderline},
		},
		{
			name: "link bold",
			in:   "[text](https://example.com)\n",
			want: "`!`[text`https://example.com]`!\n",
			opts: Options{LinkStyle: LinkBold},
		},
		{
			name: "link both",
			in:   "[text](https://example.com)\n",
			want: "`_`!`[text`https://example.com]`!`_\n",
			opts: Options{LinkStyle: LinkBoth},
		},
		{
			name: "link color 3",
			in:   "[text](https://example.com)\n",
			want: "`F00f`[text`https://example.com]`f\n",
			opts: Options{LinkStyle: LinkColor, LinkColor: "00f"},
		},
		{
			name: "link color 6",
			in:   "[text](https://example.com)\n",
			want: "`FT112233`[text`https://example.com]`f\n",
			opts: Options{LinkStyle: LinkColor, LinkColor: "112233"},
		},
		{
			name: "link color invalid falls back",
			in:   "[text](https://example.com)\n",
			want: "`[text`https://example.com]\n",
			opts: Options{LinkStyle: LinkColor, LinkColor: "red"},
		},
		{
			name: "autolink url",
			in:   "<https://example.com/x>\n",
			want: "`[https://example.com/x`https://example.com/x]\n",
		},
		{
			name: "autolink email",
			in:   "<a@b.com>\n",
			want: "`[a@b.com`mailto:a@b.com]\n",
		},
		{
			name: "image link",
			in:   "![alt text](pic.webp)\n",
			want: "`[alt text`pic.webp`img=1]\n",
		},
		{
			name: "image alt policy",
			in:   "![alt text](pic.webp)\n",
			want: "alt text\n",
			opts: Options{ImagePolicy: ImageAlt},
		},
		{
			name: "image skip policy",
			in:   "before ![alt](pic.webp) after\n",
			want: "before  after\n",
			opts: Options{ImagePolicy: ImageSkip},
		},
		{
			name: "image empty alt uses url label",
			in:   "![](pic.webp)\n",
			want: "`[pic.webp`pic.webp`img=1]\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := convert(t, tc.in, tc.opts)
			if got != tc.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

func TestConvertTables(t *testing.T) {
	in := "| A | B |\n|---|--:|\n| 1 | 2 |\n"
	got := convert(t, in, Options{MaxWidth: 100})
	want := "┌─────┬─────┐\n" +
		"│ A   │ B   │\n" +
		"├─────┼─────┤\n" +
		"│ 1   │   2 │\n" +
		"└─────┴─────┘\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestConvertTableAlign(t *testing.T) {
	in := "| A |\n|---|\n| 1 |\n"
	got := convert(t, in, Options{TableAlign: "c"})
	if !strings.HasPrefix(got, "`c\n") || !strings.HasSuffix(strings.TrimRight(got, "\n"), "`a") {
		t.Fatalf("expected align wrapper, got:\n%s", got)
	}
}

func TestConvertEscaping(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "backtick escaped",
			in:   "a ` b\n",
			want: "a \\` b\n",
		},
		{
			name: "backslash doubled",
			in:   "a \\ b\n",
			want: "a \\\\ b\n",
		},
		{
			name: "leading hash escaped",
			in:   "para\n\n#not heading?\n",
			want: "para\n\n\\#not heading?\n",
		},
		{
			name: "leading dash escaped",
			in:   "para\n\n-dash text\n",
			want: "para\n\n\\-dash text\n",
		},
		{
			name: "leading angle escaped",
			in:   "para\n\n<br> tag\n",
			want: "para\n\n\\<br> tag\n",
		},
		{
			name: "literal toggle line in code",
			in:   "```\n`=\ninside\n`=\n```\n",
			want: "`BT282828`Fddd\n`=\n\\`=\ninside\n\\`=\n`=\n`f`b\n",
		},
		{
			name: "backtick bracket sequence safe",
			in:   "text `[ not a link\n",
			want: "text \\`[ not a link\n",
		},
		{
			name: "tricky backslash backtick",
			in:   "\\`\n",
			want: "\\\\\\`\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := convert(t, tc.in, Options{})
			if got != tc.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

func TestRewriteLink(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"page.md", ":/page/page"},
		{"/abs/path", ":/page/abs/path"},
		{"sub/dir.md#sec", ":/page/sub/dir|anchor=sec"},
		{"#local", "#local"},
		{":/page/other.mu", ":/page/other.mu"},
		{"https://example.com/x", "https://example.com/x"},
		{"https://example.com/x#f", "https://example.com/x"},
		{"mailto:a@b.com", "mailto:a@b.com"},
		{":/page/x.mu#f", ":/page/x.mu|anchor=f"},
	}
	for _, tc := range cases {
		got := rewriteLink(tc.in, ":/page/", "")
		if got != tc.want {
			t.Fatalf("rewriteLink(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRewriteLinkNodeHash(t *testing.T) {
	got := rewriteLink("page.md", ":/page/", "abcdef0123456789abcdef0123456789")
	want := "abcdef0123456789abcdef0123456789:/page/page"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWrapText(t *testing.T) {
	got := wrapText("one two three four five", 10)
	want := []string{"one two", "three four", "five"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWrapTextKeepsLinksAtomic(t *testing.T) {
	link := "`[a b`https://x]"
	got := wrapText("see "+link+" end", 8)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, link) {
		t.Fatalf("link was split: %v", got)
	}
}
