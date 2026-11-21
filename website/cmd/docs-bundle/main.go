// Command docs-bundle generates downloadable copies of the docs in three
// formats: a ZIP of the Markdown sources, a single EPUB 3 e-book, and a
// single PDF rendered from a combined print document with headless
// Chromium.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const repoBlobBase = "https://github.com/Quad4-Software/Reticulum-Go/blob/master/"

var errUsage = fmt.Errorf("usage")

func usage() {
	fmt.Fprintf(os.Stderr, `docs-bundle - generate downloadable docs bundles

Flags:
`)
	flag.PrintDefaults()
}

func main() {
	siteDir := flag.String("site", "site", "Hugo site directory")
	docsDir := flag.String("docs", "../docs/en", "docs source directory")
	changelog := flag.String("changelog", "../CHANGELOG.md", "repo changelog used for the bundle version")
	outDir := flag.String("out", "", "output directory (default <site>/static/downloads)")
	requirePDF := flag.Bool("require-pdf", false, "fail when no Chromium binary is available for the PDF")
	chromium := flag.String("chromium", "", "Chromium or Chrome binary path (default: search PATH)")
	flag.Usage = usage
	flag.Parse()

	if *outDir == "" {
		*outDir = filepath.Join(*siteDir, "static", "downloads")
	}
	if err := run(*siteDir, *docsDir, *changelog, *outDir, *chromium, *requirePDF); err != nil {
		fmt.Fprintf(os.Stderr, "docs-bundle: %v\n", err)
		os.Exit(1)
	}
}

func run(siteDir, docsDir, changelogPath, outDir, chromium string, requirePDF bool) error {
	nav, _, err := loadNav(filepath.Join(siteDir, "data", "docsnav.toml"))
	if err != nil {
		return err
	}

	changelog, err := os.ReadFile(changelogPath) // #nosec G304 -- flag supplied path
	if err != nil {
		return fmt.Errorf("read changelog: %w", err)
	}
	version := docVersion(string(changelog))

	docs, err := loadDocs(docsDir, nav)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	base := "reticulum-go-docs-" + version

	zipPath := filepath.Join(outDir, base+".zip")
	if err := writeMarkdownZip(zipPath, docs); err != nil {
		return err
	}
	if err := copyFile(zipPath, filepath.Join(outDir, "reticulum-go-docs.zip")); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", zipPath)

	printHTML, err := buildPrintHTML(docs, version)
	if err != nil {
		return err
	}
	printPath := filepath.Join(outDir, base+".print.html")
	if err := os.WriteFile(printPath, []byte(printHTML), 0o644); err != nil {
		return err
	}

	epubPath := filepath.Join(outDir, base+".epub")
	if err := writeEPUB(epubPath, docs, version); err != nil {
		return err
	}
	if err := copyFile(epubPath, filepath.Join(outDir, "reticulum-go-docs.epub")); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", epubPath)

	browser := findChromium(chromium)
	if browser == "" {
		msg := "no chromium or chrome binary found, skipping PDF"
		if requirePDF {
			return fmt.Errorf("%s", msg)
		}
		fmt.Println(msg)
	} else {
		pdfPath := filepath.Join(outDir, base+".pdf")
		if err := renderPDF(browser, printPath, pdfPath); err != nil {
			return err
		}
		if err := copyFile(pdfPath, filepath.Join(outDir, "reticulum-go-docs.pdf")); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", pdfPath)
	}
	return nil
}

// doc is one documentation page in bundle order.
type doc struct {
	Slug   string
	Label  string
	Title  string
	Source string
}

// loadDocs reads every nav entry's Markdown source in order.
func loadDocs(dir string, nav []navEntry) ([]doc, error) {
	var docs []doc
	for _, e := range nav {
		path := filepath.Join(dir, e.Slug+".md")
		data, err := os.ReadFile(path) // #nosec G304 -- nav supplied path
		if err != nil {
			return nil, fmt.Errorf("read doc %s: %w", e.Slug, err)
		}
		src := string(data)
		title := e.Label
		if title == "" {
			title = docTitle(e.Slug, src)
		}
		docs = append(docs, doc{Slug: e.Slug, Label: e.Label, Title: title, Source: src})
	}
	return docs, nil
}

// findChromium returns the first usable browser binary.
func findChromium(flagPath string) string {
	if flagPath != "" {
		if _, err := exec.LookPath(flagPath); err == nil {
			return flagPath
		}
		if _, err := os.Stat(flagPath); err == nil {
			return flagPath
		}
		return ""
	}
	for _, name := range []string{
		"chromium", "chromium-browser", "google-chrome",
		"google-chrome-stable", "chrome", "headless_shell",
	} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// renderPDF prints the combined HTML file to PDF with headless Chromium.
func renderPDF(browser, htmlPath, pdfPath string) error {
	abs, err := filepath.Abs(htmlPath)
	if err != nil {
		return err
	}
	url := "file://" + abs
	cmd := exec.Command(browser, // #nosec G204 -- fixed flag set, operator supplied binary
		"--headless", "--no-sandbox", "--disable-gpu",
		"--no-pdf-header-footer",
		"--print-to-pdf="+pdfPath,
		url)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("render pdf with %s: %w", browser, err)
	}
	return nil
}

// copyFile copies src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src) // #nosec G304 -- generated path
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// mdLink maps a Markdown link destination for the combined documents.
// kind is "anchor" for the print document (doc slug -> #doc-slug) or
// "epub" (doc slug -> slug.xhtml). Links to .md files outside the bundle
// or climbing out of docs/en become repository blob URLs. Everything else
// is unchanged.
func mdLink(dest, kind string, slugs map[string]bool) string {
	if strings.Contains(dest, "://") || strings.HasPrefix(dest, "#") ||
		strings.HasPrefix(dest, "mailto:") || strings.HasPrefix(dest, "/") {
		return dest
	}
	name := dest
	if i := strings.IndexByte(name, '#'); i >= 0 {
		name = name[:i]
	}
	if name == "" {
		return dest
	}
	docRef := func(slug string) string {
		if kind == "epub" {
			return slug + ".xhtml"
		}
		return "#doc-" + slug
	}
	if strings.Contains(name, "..") {
		rel := cleanRepoPath(name)
		if rel == "" {
			return dest
		}
		if rest, ok := strings.CutPrefix(rel, "docs/en/"); ok {
			if slug, ok2 := trimMD(rest); ok2 && slugs[slug] {
				return docRef(slug)
			}
		}
		return repoBlobBase + rel
	}
	if slug, ok := trimMD(name); ok {
		if slugs[slug] {
			return docRef(slug)
		}
		return repoBlobBase + "docs/en/" + name
	}
	return dest
}

// trimMD strips a .md or .markdown suffix and reports whether one was
// present.
func trimMD(name string) (string, bool) {
	if s, ok := strings.CutSuffix(name, ".markdown"); ok {
		return s, true
	}
	if s, ok := strings.CutSuffix(name, ".md"); ok {
		return s, true
	}
	return name, false
}

// cleanRepoPath resolves ../ segments in a docs-relative link against
// docs/en. It returns the repository relative path, or "" when the link
// escapes the repository root.
func cleanRepoPath(name string) string {
	parts := strings.FieldsFunc("docs/en/"+name, func(r rune) bool { return r == '/' })
	var stack []string
	for _, p := range parts {
		switch p {
		case "..":
			if len(stack) == 0 {
				return ""
			}
			stack = stack[:len(stack)-1]
		case ".":
		default:
			stack = append(stack, p)
		}
	}
	return strings.Join(stack, "/")
}
