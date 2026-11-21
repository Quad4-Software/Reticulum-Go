package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Quad4-Software/hugo-reticulum/internal/markdown"
)

// testResolver builds a Resolver over a fake pages tree.
func testResolver(t *testing.T) *Resolver {
	t.Helper()
	dir := t.TempDir()
	for _, p := range []string{
		"index.mu",
		"about/index.mu",
		"posts/index.mu",
		"posts/first-post/index.mu",
		"posts/second-post/index.mu",
		"images/pic.txt",
	} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &Resolver{PublicDir: dir}
}

func chromeOpts(r *Resolver) Options {
	return Options{
		Resolver:    r,
		LinkStyle:   markdown.LinkBoth,
		ImagePolicy: markdown.ImageLink,
	}
}

func TestRewriteDocumentLinks(t *testing.T) {
	r := testResolver(t)
	opts := chromeOpts(r)

	in := "`[Home`/]\n" +
		"`[About`/about/]\n" +
		"`[Post`/posts/first-post/index.mu]\n" +
		"`[Ext`https://example.com/x]\n" +
		"`[Anchor`/about/#top]\n" +
		"`[Scoped`:/page/index.mu]\n" +
		"`[Pic`/images/pic.txt`img=1]\n"

	got := string(rewriteDocumentLinks([]byte(in), opts, "index"))
	want := "`_`!`[Home`:/page/index.mu]`!`_\n" +
		"`_`!`[About`:/page/about/index.mu]`!`_\n" +
		"`_`!`[Post`:/page/posts/first-post/index.mu]`!`_\n" +
		"`[Ext`https://example.com/x]\n" +
		"`_`!`[Anchor`:/page/about/index.mu|anchor=top]`!`_\n" +
		"`[Scoped`:/page/index.mu]\n" +
		"`_`!`[Pic`:/file/images/pic.txt`img=1]`!`_\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRewriteDocumentLinksLiteralAndTable(t *testing.T) {
	r := testResolver(t)
	opts := chromeOpts(r)

	in := "`=\n" +
		"`[not a link`/]\n" +
		"`=\n" +
		"`t\n" +
		"`[also not`/]\n" +
		"`t\n" +
		"`[Home`/]\n"

	got := string(rewriteDocumentLinks([]byte(in), opts, "index"))
	want := "`=\n" +
		"`[not a link`/]\n" +
		"`=\n" +
		"`t\n" +
		"`[also not`/]\n" +
		"`t\n" +
		"`_`!`[Home`:/page/index.mu]`!`_\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRewriteDocumentLinksEscapesAndComments(t *testing.T) {
	r := testResolver(t)
	opts := chromeOpts(r)

	in := "# `[comment`/]\n" +
		"\\`[escaped`/]\n" +
		"text \\` more `[Home`/] end\n"

	got := string(rewriteDocumentLinks([]byte(in), opts, "index"))
	want := "# `[comment`/]\n" +
		"\\`[escaped`/]\n" +
		"text \\` more `_`!`[Home`:/page/index.mu]`!`_ end\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRewriteDocumentLinksNoResolver(t *testing.T) {
	in := "`[Home`/]\n"
	out := rewriteDocumentLinks([]byte(in), Options{}, "index")
	if string(out) != in {
		t.Fatalf("got %q want %q", out, in)
	}
}

// TestBuiltPagesLinks audits every recorded golden page: no link may point
// at a bare "/" path, and every :/page/ link must target a .mu file.
func TestBuiltPagesLinks(t *testing.T) {
	root := filepath.Join("testdata", "example")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".mu") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, problem := range auditLinks(string(data)) {
			t.Errorf("%s: %s", filepath.ToSlash(path), problem)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// auditLinks returns a description for every link that violates the built
// page link rules.
func auditLinks(doc string) []string {
	var problems []string
	literal, table := false, false
	for _, line := range strings.Split(doc, "\n") {
		if strings.Trim(line, " \t\r") == "`=" {
			literal = !literal
			continue
		}
		if literal {
			continue
		}
		if strings.HasPrefix(line, "`t") {
			table = !table
			continue
		}
		if table || strings.HasPrefix(line, "#") {
			continue
		}
		for i := 0; i+1 < len(line); i++ {
			if line[i] == '\\' && i+1 < len(line) && isMicronEscape(line[i+1]) {
				i++
				continue
			}
			if line[i] != '`' || line[i+1] != '[' {
				continue
			}
			link, next, ok := parseMicronLink(line, i+1)
			if !ok {
				continue
			}
			i = next - 1
			url := strings.SplitN(link.url, "|", 2)[0]
			switch {
			case strings.HasPrefix(url, "/"):
				problems = append(problems, "bare path link "+link.url)
			case strings.Contains(url, ":/page/"):
				if !strings.HasSuffix(url, ".mu") {
					problems = append(problems, "page link without .mu: "+link.url)
				}
			case strings.Contains(url, ":/"):
				// :/file/ and other scopes are fine.
			case strings.Contains(url, "://") || strings.HasPrefix(url, "#") ||
				strings.HasPrefix(url, "mailto:") || strings.HasPrefix(url, "tel:"):
				// External or anchor, fine.
			default:
				problems = append(problems, "unscoped link "+link.url)
			}
		}
	}
	return problems
}

func TestRewriteDocumentLinksBareForm(t *testing.T) {
	r := testResolver(t)
	opts := chromeOpts(r)

	in := "`[/about/]`[x`y]\n"
	got := string(rewriteDocumentLinks([]byte(in), opts, "index"))
	want := "`_`!`[:/page/about/index.mu]`!`_`_`!`[x`:/page/y/index.mu]`!`_\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
