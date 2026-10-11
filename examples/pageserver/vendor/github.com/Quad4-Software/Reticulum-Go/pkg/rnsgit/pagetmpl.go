// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Template substitution keys, matching the placeholders rngit template files
// use. Only base sees the full set. Per-page templates get PAGE_CONTENT.
const (
	tplPageContent = "{PAGE_CONTENT}"
	tplNodeName    = "{NODE_NAME}"
	tplVersion     = "{VERSION}"
	tplNavigation  = "{NAVIGATION}"
	tplGenTime     = "{GEN_TIME}"
)

// Version is the rngit version string shown in page footers. The CLI stamps
// it from the build version. Library users may set it before serving.
var Version = "dev"

// tabWidth matches the Python format_tabs replacement width.
const tabWidth = 3

// templateExecTimeout bounds dynamic template execution. The Python
// implementation runs executable templates with no timeout or output cap.
// both bounds are deliberate hardening additions here.
const (
	templateExecTimeout = 5 * time.Second
	templateExecMaxOut  = 1 << 20
)

// Default page templates. Everything except base and front is a plain content
// injection point, so custom templates are the supported customization path.
var defaultTemplates = map[string]string{
	"base": "#!c=0\n" +
		"> {NODE_NAME}\n\n" +
		"{NAVIGATION}\n" +
		"{PAGE_CONTENT}\n" +
		"<\n-\n" +
		"`a`F666`[Served by Reticulum-Go rngit {VERSION}`:/page/index.mu] - {GEN_TIME}`f",
	"front":    "> Groups\n\n{PAGE_CONTENT}",
	"group":    "{PAGE_CONTENT}",
	"repo":     "{PAGE_CONTENT}",
	"releases": "{PAGE_CONTENT}",
	"release":  "{PAGE_CONTENT}",
	"tree":     "{PAGE_CONTENT}",
	"blob":     "{PAGE_CONTENT}",
	"commits":  "{PAGE_CONTENT}",
	"commit":   "{PAGE_CONTENT}",
	"refs":     "{PAGE_CONTENT}",
	"stats":    "{PAGE_CONTENT}",
	"work":     "{PAGE_CONTENT}",
	"work_doc": "{PAGE_CONTENT}",
}

// templateCacheEntry caches a template file by modification fingerprint so
// live edits still apply without re-reading unchanged files each request.
type templateCacheEntry struct {
	modTime time.Time
	size    int64
	content string
}

// templateCache caches static template file contents keyed by path.
type templateCache struct {
	mu      sync.Mutex
	entries map[string]templateCacheEntry
}

func (c *templateCache) get(path string, st os.FileInfo) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]templateCacheEntry{}
	}
	if e, ok := c.entries[path]; ok && e.modTime.Equal(st.ModTime()) && e.size == st.Size() {
		return e.content, true
	}
	return "", false
}

func (c *templateCache) put(path string, st os.FileInfo, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]templateCacheEntry{}
	}
	c.entries[path] = templateCacheEntry{modTime: st.ModTime(), size: st.Size(), content: content}
}

// getTemplate loads a custom template from <configdir>/templates/<name>.mu.
// Executable files are run and their stdout becomes the template, matching
// the Python dynamic template behavior with added bounds. Returns "" when no
// custom template exists.
func (n *Node) getTemplate(name string) string {
	if n.cfg == nil || n.cfg.ConfigDir == "" || name == "" {
		return ""
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return ""
	}
	path := filepath.Join(n.cfg.ConfigDir, "templates", name+".mu")
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return ""
	}
	if st.Mode()&0o111 != 0 {
		return runTemplateExec(path)
	}
	if cached, ok := n.tmplCache.get(path, st); ok {
		return cached
	}
	b, err := os.ReadFile(path) // #nosec G304 -- operator template path
	if err != nil {
		return ""
	}
	content := strings.TrimRight(string(b), " \t\r\n")
	n.tmplCache.put(path, st, content)
	return content
}

// runTemplateExec executes a dynamic template file with bounded time and
// output, returning trimmed stdout.
func runTemplateExec(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), templateExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path) // #nosec G204 -- operator-provided template executable
	// WaitDelay bounds the post-kill wait for stdout copiers, so a child that
	// orphans a grandchild holding the pipe cannot stall the request.
	cmd.WaitDelay = time.Second
	var buf strings.Builder
	buf.Grow(4096)
	cmd.Stdout = &boundedWriter{w: &buf, max: templateExecMaxOut}
	if err := cmd.Run(); err != nil || ctx.Err() != nil {
		return ""
	}
	return strings.TrimRight(buf.String(), " \t\r\n")
}

// boundedWriter drops output beyond max bytes.
type boundedWriter struct {
	w   *strings.Builder
	max int
}

func (b *boundedWriter) Write(p []byte) (int, error) {
	remaining := b.max - b.w.Len()
	if remaining > 0 {
		if len(p) > remaining {
			b.w.Write(p[:remaining])
		} else {
			b.w.Write(p)
		}
	}
	return len(p), nil
}

// formatTabs expands tab characters in generated content, matching Python
// format_tabs. Custom template text is not rewritten.
func formatTabs(s string) string {
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth))
}

// renderTemplate produces the final micron page bytes. It applies the
// per-page template first, then the base template, matching the two-stage
// substitution in the Python renderer.
func (n *Node) renderTemplate(pageContent, nav, name string, st time.Time) []byte {
	content := formatTabs(pageContent)
	if name != "" {
		tmpl := n.getTemplate(name)
		if tmpl == "" {
			tmpl = defaultTemplates[name]
		}
		if tmpl != "" {
			content = strings.ReplaceAll(tmpl, tplPageContent, content)
		}
	}
	base := n.getTemplate("base")
	if base == "" {
		base = defaultTemplates["base"]
	}
	gen := "Unknown generation time"
	if !st.IsZero() {
		gen = "Generated in " + prettyTime(time.Since(st).Seconds())
	}
	out := strings.ReplaceAll(base, tplNodeName, n.cfg.NodeName)
	out = strings.ReplaceAll(out, tplVersion, Version)
	out = strings.ReplaceAll(out, tplNavigation, nav)
	out = strings.ReplaceAll(out, tplGenTime, gen)
	out = strings.ReplaceAll(out, tplPageContent, content)
	return []byte(out)
}

// renderPage is the common entry point: build content and nav through fn and
// emit the templated page.
func (n *Node) renderPage(name, nav string, st time.Time, body func(b *strings.Builder)) []byte {
	var b strings.Builder
	b.Grow(8192)
	body(&b)
	return n.renderTemplate(b.String(), nav, name, st)
}

// pageError renders a uniform error page body.
func pageError(msg string) string {
	return ">Notice\n\n" + mEscape(msg) + "\n"
}

func fmtInt(n int) string {
	return fmt.Sprintf("%d", n)
}
