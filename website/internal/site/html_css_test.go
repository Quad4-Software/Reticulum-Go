package site

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestCodeblockSingleSurface guards the code block styling contract: the
// .codeblock container is the only element that paints a surface
// (background, border, radius), while the nested .highlight wrapper,
// pre and code stay transparent. A second surface is what produced the
// "double box" rendering.
func TestCodeblockSingleSurface(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("..", "..", "site", "static", "css", "site.css"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(css)

	rule := func(sel string) string {
		re := regexp.MustCompile(regexp.QuoteMeta(sel) + `\s*\{([^}]*)\}`)
		m := re.FindStringSubmatch(text)
		if m == nil {
			t.Fatalf("rule %q not found", sel)
		}
		return m[1]
	}

	box := rule(".codeblock")
	for _, want := range []string{"background:", "border:", "border-radius:", "overflow-x: auto"} {
		if !strings.Contains(box, want) {
			t.Errorf(".codeblock rule missing %q", want)
		}
	}

	inner := rule(".codeblock .highlight,\n.codeblock .chroma,\n.codeblock pre,\n.codeblock pre.chroma")
	if inner == "" {
		inner = rule(".codeblock .highlight, .codeblock .chroma, .codeblock pre, .codeblock pre.chroma")
	}
	for _, want := range []string{"background: none", "border: 0", "border-radius: 0"} {
		if !strings.Contains(inner, want) {
			t.Errorf("inner code elements missing %q", want)
		}
	}
}
