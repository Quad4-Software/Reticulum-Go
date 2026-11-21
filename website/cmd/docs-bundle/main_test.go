package main

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testNav = `
[[groups]]
name = "Start"
  [[groups.pages]]
  slug = "overview"
  label = "Overview"
  [[groups.pages]]
  slug = "getting-started"
  label = "Getting started"
[[groups]]
name = "Internals"
  [[groups.pages]]
  slug = "transport"
  label = "Transport"
`

func TestLoadNavOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docsnav.toml")
	if err := os.WriteFile(path, []byte(testNav), 0o644); err != nil {
		t.Fatal(err)
	}
	nav, _, err := loadNav(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"overview", "getting-started", "transport"}
	if len(nav) != len(want) {
		t.Fatalf("nav len %d want %d", len(nav), len(want))
	}
	for i, e := range nav {
		if e.Slug != want[i] {
			t.Fatalf("nav[%d] = %q want %q", i, e.Slug, want[i])
		}
	}
}

func TestDocVersion(t *testing.T) {
	cl := "# Changelog\n\n## v1.6.2 - unreleased\n\n- wip\n\n## v1.6.1 - 2026-11-01\n\n- stuff\n\n## 1.6.0\n"
	if v := docVersion(cl); v != "1.6.1" {
		t.Fatalf("docVersion = %q want 1.6.1", v)
	}
	if v := docVersion("## v1.6.2 - unreleased\n"); v != "unreleased" {
		t.Fatalf("docVersion = %q want unreleased", v)
	}
}

func TestDocTitle(t *testing.T) {
	if got := docTitle("x", "# Some doc\n\ntext\n"); got != "Some doc" {
		t.Fatalf("docTitle = %q", got)
	}
	if got := docTitle("x", "no heading\n"); got != "x" {
		t.Fatalf("docTitle fallback = %q", got)
	}
}

func TestMdLink(t *testing.T) {
	slugs := map[string]bool{"overview": true, "transport": true}
	cases := []struct{ dest, want string }{
		{"overview.md", "#doc-overview"},
		{"transport.md#paths", "#doc-transport"},
		{"../../SECURITY.md", repoBlobBase + "SECURITY.md"},
		{"../../COMPATIBILITY.md", repoBlobBase + "COMPATIBILITY.md"},
		{"../../install.sh", repoBlobBase + "install.sh"},
		{"README.md", repoBlobBase + "docs/en/README.md"},
		{"#local", "#local"},
		{"https://example.com/x", "https://example.com/x"},
		{"/docs/overview/", "/docs/overview/"},
		{"image.png", "image.png"},
	}
	for _, c := range cases {
		if got := mdLink(c.dest, "anchor", slugs); got != c.want {
			t.Errorf("mdLink(%q) = %q want %q", c.dest, got, c.want)
		}
	}
	if got := mdLink("overview.md", "epub", slugs); got != "overview.xhtml" {
		t.Errorf("epub mdLink = %q", got)
	}
}

func TestRewriteDocLinks(t *testing.T) {
	src := "# Doc\n\nSee [a](other.md) and `[not](code.md)`.\n\n```\n[skip](skip.md)\n```\n\nAlso [b](../../LICENSE).\n"
	out := rewriteDocLinks(src, func(dest string) string {
		return "X:" + dest
	})
	if !strings.Contains(out, "[a](X:other.md)") {
		t.Fatalf("inline link not rewritten:\n%s", out)
	}
	if !strings.Contains(out, "`[not](code.md)`") {
		t.Fatalf("code span rewritten:\n%s", out)
	}
	if !strings.Contains(out, "[skip](skip.md)") {
		t.Fatalf("fence rewritten:\n%s", out)
	}
	if !strings.Contains(out, "[b](X:../../LICENSE)") {
		t.Fatalf("dotdot link not rewritten:\n%s", out)
	}
}

func testDocs() []doc {
	return []doc{
		{Slug: "overview", Label: "Overview", Title: "Overview", Source: "# Overview\n\nIntro [gs](getting-started.md).\n"},
		{Slug: "getting-started", Label: "Getting started", Title: "Getting started", Source: "# Getting started\n\n```sh\nmake build\n```\n"},
	}
}

func TestWriteMarkdownZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docs.zip")
	if err := writeMarkdownZip(path, testDocs()); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"INDEX.md", "overview.md", "getting-started.md"} {
		if !names[want] {
			t.Fatalf("zip missing %s, has %v", want, names)
		}
	}
}

func TestWriteEPUBStructure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docs.epub")
	if err := writeEPUB(path, testDocs(), "1.6.1"); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) == 0 || zr.File[0].Name != "mimetype" {
		t.Fatalf("first zip entry is not mimetype")
	}
	if zr.File[0].Method != zip.Store {
		t.Fatalf("mimetype is compressed")
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 20)
	n, _ := rc.Read(buf)
	rc.Close()
	if string(buf[:n]) != "application/epub+zip" {
		t.Fatalf("mimetype content %q", buf[:n])
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{
		"META-INF/container.xml", "OEBPS/content.opf", "OEBPS/nav.xhtml",
		"OEBPS/style.css", "OEBPS/overview.xhtml", "OEBPS/getting-started.xhtml",
	} {
		if !names[want] {
			t.Fatalf("epub missing %s", want)
		}
	}
	// The nav and opf must parse as XML and list every document.
	for _, name := range []string{"OEBPS/content.opf", "OEBPS/nav.xhtml", "OEBPS/overview.xhtml"} {
		for _, f := range zr.File {
			if f.Name == name {
				rc, _ := f.Open()
				data, _ := io.ReadAll(rc)
				rc.Close()
				var v any
				if err := xml.Unmarshal(data, &v); err != nil {
					t.Fatalf("%s is not valid XML: %v", name, err)
				}
				if name == "OEBPS/nav.xhtml" && !strings.Contains(string(data), `epub:type="toc"`) {
					t.Fatalf("nav.xhtml missing toc nav")
				}
			}
		}
	}
}
