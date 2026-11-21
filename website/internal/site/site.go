// Package site post-processes Hugo Micron output. Templates emit Markdown
// bodies wrapped in <!--md--> sentinel lines. This package converts those
// regions to Micron and rewrites links against the built site.
package site

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Quad4-Software/hugo-reticulum/internal/markdown"
)

const (
	// SentinelOpen and SentinelClose delimit Markdown body regions in
	// template output.
	SentinelOpen  = "<!--md-->"
	SentinelClose = "<!--/md-->"
)

// Options controls sentinel processing.
type Options struct {
	// Resolver rewrites link and image destinations. Nil leaves them to the
	// markdown defaults.
	Resolver *Resolver

	// Markdown options applied to each sentinel region.
	MaxWidth    int
	LinkStyle   markdown.LinkStyle
	LinkColor   string
	ImagePolicy markdown.ImagePolicy
	TableAlign  string
}

// markdownOptions builds the converter options for one page.
func (o *Options) markdownOptions(pagePath string) markdown.Options {
	mo := markdown.Options{
		MaxWidth:    o.MaxWidth,
		LinkStyle:   o.LinkStyle,
		LinkColor:   o.LinkColor,
		ImagePolicy: o.ImagePolicy,
		TableAlign:  o.TableAlign,
	}
	if o.Resolver != nil {
		mo.RewriteLink = func(url string) string {
			return o.Resolver.Link(url, pagePath)
		}
		mo.RewriteImageLink = func(url string) string {
			return o.Resolver.Image(url, pagePath)
		}
	}
	return mo
}

// ProcessBytes converts sentinel regions in data and rewrites site links
// in the Micron chrome. pagePath is the slash path of the page relative to
// the pages root, without the .mu suffix, and is used for relative link
// resolution. It may be empty.
func ProcessBytes(data []byte, opts Options, pagePath string) ([]byte, error) {
	return process(data, opts, pagePath)
}

// HasSentinels reports whether data contains a sentinel region.
func HasSentinels(data []byte) bool {
	return bytes.Contains(data, []byte(SentinelOpen))
}

// WalkFiles applies sentinel processing and chrome link rewriting to every
// .mu file under root. It returns the number of files rewritten. The page
// path for link resolution is each file's slash path relative to root,
// without the .mu suffix.
func WalkFiles(root string, opts Options) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".mu") {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 -- walked path
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pagePath := strings.TrimSuffix(filepath.ToSlash(rel), ".mu")
		out, err := process(data, opts, pagePath)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

// RenderTree copies every .mu file under srcRoot to dstRoot, converting
// sentinel regions and rewriting links along the way. It returns the number
// of files written. The page path for link resolution is each file's slash
// path relative to srcRoot, without the .mu suffix.
func RenderTree(srcRoot, dstRoot string, opts Options) (int, error) {
	count := 0
	err := filepath.WalkDir(srcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".mu") {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 -- walked path
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		pagePath := strings.TrimSuffix(filepath.ToSlash(rel), ".mu")
		out, err := process(data, opts, pagePath)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		dst := filepath.Join(dstRoot, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, out, 0o644); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

// process converts sentinel regions using the site's markdown options, then
// rewrites site relative links in the Micron chrome.
func process(data []byte, opts Options, pagePath string) ([]byte, error) {
	mo := opts.markdownOptions(pagePath)
	lines := bytes.Split(data, []byte("\n"))
	// A trailing newline produces a final empty element; do not emit an
	// extra blank line for it.
	if n := len(lines); n > 0 && len(lines[n-1]) == 0 {
		lines = lines[:n-1]
	}
	var out bytes.Buffer
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(string(lines[i]), "\r")
		if strings.TrimSpace(line) != SentinelOpen {
			out.Write(lines[i])
			out.WriteByte('\n')
			continue
		}
		var body bytes.Buffer
		closed := false
		for i++; i < len(lines); i++ {
			l := strings.TrimRight(string(lines[i]), "\r")
			if strings.TrimSpace(l) == SentinelClose {
				closed = true
				break
			}
			body.Write(lines[i])
			body.WriteByte('\n')
		}
		if !closed {
			return nil, fmt.Errorf("unclosed %s sentinel", SentinelOpen)
		}
		converted, err := markdown.Convert(body.String(), mo)
		if err != nil {
			return nil, err
		}
		out.WriteString(converted)
	}
	return rewriteDocumentLinks(out.Bytes(), opts, pagePath), nil
}
