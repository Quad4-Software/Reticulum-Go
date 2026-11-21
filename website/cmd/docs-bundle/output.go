package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"os"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// goldmark instances for the two output flavors.
var (
	mdHTML = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	mdXHTML = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(gmhtml.WithXHTML()),
	)
)

// convert renders Markdown to HTML with intra-bundle links rewritten.
func convert(md goldmark.Markdown, src, kind string, slugs map[string]bool) (string, error) {
	src = rewriteDocLinks(src, func(dest string) string {
		return mdLink(dest, kind, slugs)
	})
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func slugSet(docs []doc) map[string]bool {
	set := make(map[string]bool, len(docs))
	for _, d := range docs {
		set[d.Slug] = true
	}
	return set
}

// writeMarkdownZip writes the Markdown sources plus an INDEX.md that lists
// the documents in navigation order.
func writeMarkdownZip(path string, docs []doc) error {
	f, err := os.Create(path) // #nosec G304 -- generated path
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)

	var index strings.Builder
	index.WriteString("# Reticulum-Go documentation\n\n")
	index.WriteString("Documents in reading order.\n\n")
	for _, d := range docs {
		fmt.Fprintf(&index, "- %s - %s.md\n", d.Title, d.Slug)
	}
	if err := zipWrite(zw, "INDEX.md", index.String()); err != nil {
		f.Close()
		return err
	}
	for _, d := range docs {
		if err := zipWrite(zw, d.Slug+".md", d.Source); err != nil {
			f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func zipWrite(zw *zip.Writer, name, content string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, content)
	return err
}

// printCSS styles the combined print document.
const printCSS = `
@page { margin: 20mm 16mm; @bottom-center { content: counter(page); color: #666; font: 9pt sans-serif; } }
* { box-sizing: border-box; }
body { margin: 0 auto; max-width: 46rem; padding: 0 1rem; color: #1f2328;
  font: 10.5pt/1.6 -apple-system, "Segoe UI", Roboto, "DejaVu Sans", sans-serif; }
h1, h2, h3, h4 { line-height: 1.3; page-break-after: avoid; }
h1 { font-size: 20pt; } h2 { font-size: 14pt; border-bottom: 1px solid #d8dde2; padding-bottom: .2em; }
h3 { font-size: 12pt; }
.title-page { page-break-after: always; padding-top: 30vh; }
.title-page h1 { font-size: 28pt; margin: 0; }
.title-page .version { color: #59636e; font-size: 12pt; }
.toc { page-break-after: always; }
.toc ol { padding-left: 1.4em; }
.toc li { margin: .25em 0; }
.chapter { page-break-before: always; }
.chapter > :first-child { margin-top: 0; }
code, pre { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "DejaVu Sans Mono", monospace; font-size: .88em; }
code { background: #eff1f3; padding: .12em .35em; border-radius: 4px; }
pre { background: #f5f6f7; border: 1px solid #d8dde2; border-radius: 6px; padding: .8em 1em;
  overflow-x: auto; white-space: pre-wrap; word-break: break-word; page-break-inside: avoid; }
pre code { background: none; padding: 0; }
table { border-collapse: collapse; width: 100%; margin: 1em 0; font-size: 9.5pt; }
th, td { border-bottom: 1px solid #d8dde2; padding: .4em .6em; text-align: left; vertical-align: top; }
th { background: #f5f6f7; }
blockquote { margin: 1em 0; padding-left: 1em; border-left: 3px solid #d8dde2; color: #59636e; }
a { color: #0a5cad; text-decoration: none; }
img { max-width: 100%; }
`

// buildPrintHTML renders the combined print document: a title page, a
// table of contents, and one chapter per document.
func buildPrintHTML(docs []doc, version string) (string, error) {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	fmt.Fprintf(&b, "<title>Reticulum-Go documentation %s</title>\n", html.EscapeString(version))
	b.WriteString("<style>\n")
	b.WriteString(printCSS)
	b.WriteString("\n</style>\n</head>\n<body>\n")

	b.WriteString("<div class=\"title-page\">\n")
	b.WriteString("<h1>Reticulum-Go documentation</h1>\n")
	fmt.Fprintf(&b, "<p class=\"version\">Version %s</p>\n", html.EscapeString(version))
	b.WriteString("</div>\n")

	b.WriteString("<nav class=\"toc\">\n<h1>Contents</h1>\n<ol>\n")
	for _, d := range docs {
		fmt.Fprintf(&b, "<li><a href=\"#doc-%s\">%s</a></li>\n", d.Slug, html.EscapeString(d.Title))
	}
	b.WriteString("</ol>\n</nav>\n")

	set := slugSet(docs)
	for _, d := range docs {
		body, err := convert(mdHTML, d.Source, "anchor", set)
		if err != nil {
			return "", fmt.Errorf("convert %s: %w", d.Slug, err)
		}
		fmt.Fprintf(&b, "<section class=\"chapter\" id=\"doc-%s\">\n%s\n</section>\n", d.Slug, body)
	}
	b.WriteString("</body>\n</html>\n")
	return b.String(), nil
}

const epubCSS = `
body { font-family: serif; line-height: 1.55; }
h1, h2, h3, h4 { line-height: 1.3; }
code, pre { font-family: monospace; font-size: .9em; }
code { background: #f0f0f0; padding: 0 .2em; }
pre { background: #f5f5f5; border: 1px solid #ddd; padding: .7em; white-space: pre-wrap; word-break: break-word; }
pre code { background: none; padding: 0; }
table { border-collapse: collapse; width: 100%; }
th, td { border-bottom: 1px solid #ccc; padding: .3em .5em; text-align: left; vertical-align: top; }
blockquote { margin-left: 0; padding-left: 1em; border-left: 3px solid #ccc; color: #555; }
`

// writeEPUB assembles a valid EPUB 3 archive: the mimetype entry first and
// stored uncompressed, then the container, package document, nav, style
// sheet, and one XHTML file per document.
func writeEPUB(path string, docs []doc, version string) error {
	f, err := os.Create(path) // #nosec G304 -- generated path
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)

	mh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, err := zw.CreateHeader(mh)
	if err != nil {
		f.Close()
		return err
	}
	if _, err := io.WriteString(mw, "application/epub+zip"); err != nil {
		f.Close()
		return err
	}

	files := []struct{ name, body string }{
		{"META-INF/container.xml", epubContainer},
		{"OEBPS/style.css", epubCSS},
	}

	set := slugSet(docs)
	type item struct{ id, href string }
	var items []item
	for _, d := range docs {
		body, err := convert(mdXHTML, d.Source, "epub", set)
		if err != nil {
			f.Close()
			return fmt.Errorf("convert %s: %w", d.Slug, err)
		}
		name := "OEBPS/" + d.Slug + ".xhtml"
		items = append(items, item{id: "doc-" + d.Slug, href: d.Slug + ".xhtml"})
		files = append(files, struct{ name, body string }{name, epubDoc(d, body)})
	}

	now := time.Now().UTC()
	opf := epubOPF(docs, version, now)
	files = append(files[:2:2], append([]struct{ name, body string }{
		{"OEBPS/content.opf", opf},
		{"OEBPS/nav.xhtml", epubNav(docs)},
	}, files[2:]...)...)

	for _, file := range files {
		if err := zipWrite(zw, file.name, file.body); err != nil {
			f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

const epubContainer = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`

func epubDoc(d doc, body string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" lang="en">
<head>
<title>%s</title>
<link rel="stylesheet" type="text/css" href="style.css"/>
</head>
<body>
%s
</body>
</html>
`, xmlEscape(d.Title), body)
}

func epubNav(docs []doc) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="en">
<head>
<title>Contents</title>
<link rel="stylesheet" type="text/css" href="style.css"/>
</head>
<body>
<nav epub:type="toc">
<h1>Contents</h1>
<ol>
`)
	for _, d := range docs {
		fmt.Fprintf(&b, "<li><a href=\"%s.xhtml\">%s</a></li>\n", d.Slug, xmlEscape(d.Title))
	}
	b.WriteString("</ol>\n</nav>\n</body>\n</html>\n")
	return b.String()
}

func epubOPF(docs []doc, version string, t time.Time) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="uid" version="3.0">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
`)
	fmt.Fprintf(&b, "<dc:identifier id=\"uid\">reticulum-go-docs-%s</dc:identifier>\n", xmlEscape(version))
	b.WriteString("<dc:title>Reticulum-Go documentation</dc:title>\n<dc:language>en</dc:language>\n")
	fmt.Fprintf(&b, "<dc:date>%s</dc:date>\n", t.Format("2006-01-02"))
	fmt.Fprintf(&b, "<meta property=\"dcterms:modified\">%s</meta>\n", t.Format("2006-01-02T15:04:05Z"))
	b.WriteString("</metadata>\n<manifest>\n")
	b.WriteString("<item id=\"nav\" href=\"nav.xhtml\" media-type=\"application/xhtml+xml\" properties=\"nav\"/>\n")
	b.WriteString("<item id=\"css\" href=\"style.css\" media-type=\"text/css\"/>\n")
	for _, d := range docs {
		fmt.Fprintf(&b, "<item id=\"doc-%s\" href=\"%s.xhtml\" media-type=\"application/xhtml+xml\"/>\n", d.Slug, d.Slug)
	}
	b.WriteString("</manifest>\n<spine>\n<itemref idref=\"nav\" linear=\"no\"/>\n")
	for _, d := range docs {
		fmt.Fprintf(&b, "<itemref idref=\"doc-%s\"/>\n", d.Slug)
	}
	b.WriteString("</spine>\n</package>\n")
	return b.String()
}

func xmlEscape(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return html.EscapeString(s)
	}
	return b.String()
}
