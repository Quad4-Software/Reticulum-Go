package site

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Resolver rewrites Markdown link and image destinations into Micron scoped
// URLs such as ":/page/about/index.mu" or "hash:/file/images/pic.webp".
type Resolver struct {
	// PublicDir is the Hugo output directory that contains the .mu tree.
	PublicDir string

	// NodeHash, when set, prefixes page and file scopes as
	// "<hash>:/page/...".
	NodeHash string

	// PageScope and FileScope override the default scopes.
	PageScope string
	FileScope string

	// RepoBaseURL, when set, rewrites relative links that escape the
	// content mount (such as ../../SECURITY.md in docs/en) to repository
	// URLs instead of guessing a page path that 404s.
	RepoBaseURL string

	// RepoDocsDir is the repository directory the content/docs mount
	// serves. Default "docs/en".
	RepoDocsDir string

	index   map[string]bool
	indexed bool
}

func (r *Resolver) pageScope() string {
	if r.PageScope != "" {
		return r.PageScope
	}
	return ":/page/"
}

func (r *Resolver) fileScope() string {
	if r.FileScope != "" {
		return r.FileScope
	}
	return ":/file/"
}

func (r *Resolver) scope(base string) string {
	if r.NodeHash != "" {
		return r.NodeHash + ":" + strings.TrimPrefix(base, ":")
	}
	return base
}

// Link rewrites a Markdown link destination for the page at fromPage.
func (r *Resolver) Link(href, fromPage string) string {
	return r.rewrite(href, fromPage, false)
}

// Image rewrites a Markdown image destination for the page at fromPage.
func (r *Resolver) Image(href, fromPage string) string {
	return r.rewrite(href, fromPage, true)
}

func (r *Resolver) rewrite(href, fromPage string, isImage bool) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return href
	}
	lower := strings.ToLower(href)
	if strings.Contains(href, "://") || strings.HasPrefix(href, "//") ||
		strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "tel:") ||
		strings.HasPrefix(href, ":/") || strings.Contains(href, ":/") {
		return href
	}

	frag := ""
	if i := strings.IndexByte(href, '#'); i >= 0 {
		frag = href[i+1:]
		href = href[:i]
	}

	if u := r.repoLink(href, fromPage); u != "" {
		if frag != "" {
			u += "#" + frag
		}
		return u
	}

	var out string
	if isImage || (hasFileExtension(href) && !isMarkdownPath(href) && !isMicronPath(href)) {
		out = r.scope(r.fileScope()) + r.filePath(href)
	} else {
		out = r.scope(r.pageScope()) + r.pagePath(href, fromPage)
	}
	if frag != "" {
		out += "|anchor=" + frag
	}
	return out
}

// pagePath resolves a link destination to a .mu path relative to the pages
// root. Markdown file links map through the Hugo URL layout, other links
// resolve against the index of built pages.
func (r *Resolver) pagePath(href, fromPage string) string {
	if isMarkdownPath(href) {
		return r.markdownPagePath(href, fromPage)
	}
	return r.lookupPage(href, fromPage)
}

// markdownPagePath maps a content-relative .md link to its output .mu path.
// Content directories do not always mirror the output tree (a leaf bundle
// like posts/first-post.md still lives in content/posts), so each ancestor
// directory of the source page is tried until a built page matches.
func (r *Resolver) markdownPagePath(href, fromPage string) string {
	if strings.HasPrefix(href, "/") {
		rel := path.Clean(strings.TrimPrefix(href, "/"))
		rel = strings.TrimSuffix(rel, ".md")
		rel = strings.TrimSuffix(rel, ".markdown")
		if p := r.matchPagePath(rel); p != "" {
			return p
		}
		return r.guessPagePath(rel)
	}
	rel := strings.TrimSuffix(href, ".md")
	rel = strings.TrimSuffix(rel, ".markdown")
	rel = path.Clean(rel)

	dir := contentDirOf(fromPage)
	for {
		candidate := rel
		if dir != "" {
			candidate = path.Join(dir, rel)
		}
		if p := r.matchPagePath(candidate); p != "" {
			return p
		}
		if dir == "" {
			break
		}
		parent := path.Dir(dir)
		if parent == "." || parent == dir {
			dir = ""
		} else {
			dir = parent
		}
	}
	candidate := rel
	if d := contentDirOf(fromPage); d != "" {
		candidate = path.Join(d, rel)
	}
	return r.guessPagePath(candidate)
}

// matchPagePath returns the .mu path for rel when the built index confirms
// it, or the path shape is certain. It returns "" when nothing matches.
func (r *Resolver) matchPagePath(rel string) string {
	rel = cleanContentPath(rel)
	if rel == "" {
		return "index.mu"
	}
	if strings.HasSuffix(rel, "/index") || rel == "index" {
		return rel + ".mu"
	}
	if r.hasPage(rel + "/index.mu") {
		return rel + "/index.mu"
	}
	if r.hasPage(rel + ".mu") {
		return rel + ".mu"
	}
	return ""
}

// guessPagePath is matchPagePath with a directory index fallback for paths
// the index does not know about.
func (r *Resolver) guessPagePath(rel string) string {
	rel = cleanContentPath(rel)
	if rel == "" {
		return "index.mu"
	}
	if strings.HasSuffix(rel, "/index") || rel == "index" {
		return rel + ".mu"
	}
	if p := r.matchPagePath(rel); p != "" {
		return p
	}
	return rel + "/index.mu"
}

// cleanContentPath normalizes a content-relative path and maps section
// index files to their directory.
func cleanContentPath(rel string) string {
	rel = path.Clean("/" + rel)
	rel = strings.TrimPrefix(rel, "/")
	switch {
	case rel == "" || rel == "." || rel == "_index":
		return ""
	case strings.HasSuffix(rel, "/_index"):
		rel = strings.TrimSuffix(rel, "/_index")
	}
	return rel
}

// lookupPage resolves a non-markdown href against the built page index.
func (r *Resolver) lookupPage(href, fromPage string) string {
	href = strings.TrimSuffix(href, "/")
	if href == "" {
		// "/" is the site root, not a link relative to the current page.
		return "index.mu"
	}
	if strings.HasSuffix(href, ".mu") {
		return strings.TrimPrefix(href, "/")
	}
	var rel string
	if strings.HasPrefix(href, "/") {
		rel = strings.TrimPrefix(href, "/")
	} else {
		rel = path.Join(outputDirOf(fromPage), href)
	}
	rel = path.Clean("/" + rel)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		rel = "index"
	}

	candidates := []string{rel + ".mu", rel + "/index.mu", rel}
	for _, c := range candidates {
		if r.hasPage(c) {
			return c
		}
	}
	// Unknown target: guess the Hugo directory index layout.
	return rel + "/index.mu"
}

// repoLink maps a ".." relative link that escapes the content mount to a
// repository URL. The docs mount serves files from RepoDocsDir, so a link
// that climbs above the mount points at a repository file rather than a
// site page. It returns "" when the link stays inside the mount, when it
// escapes the repository root, or when RepoBaseURL is unset.
func (r *Resolver) repoLink(href, fromPage string) string {
	if r.RepoBaseURL == "" || !strings.Contains(href, "..") {
		return ""
	}
	outDir := outputDirOf(fromPage)
	docsDir := r.RepoDocsDir
	if docsDir == "" {
		docsDir = "docs/en"
	}
	// outDir is the page bundle directory in the output tree, for example
	// docs/overview. Its source file lives at docsDir/<page>.md, so the
	// source directory is docsDir joined with the bundle's parent path.
	var srcDir string
	switch {
	case outDir == "docs":
		srcDir = docsDir
	case strings.HasPrefix(outDir, "docs/"):
		inner := strings.TrimPrefix(outDir, "docs")
		srcDir = path.Join(docsDir, path.Dir(inner))
	default:
		return ""
	}
	rel := path.Clean(path.Join(srcDir, href))
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return ""
	}
	return strings.TrimSuffix(r.RepoBaseURL, "/") + "/" + rel
}

// filePath normalizes a file destination to a path under the files root.
func (r *Resolver) filePath(href string) string {
	href = strings.TrimPrefix(href, "/")
	href = path.Clean("/" + href)
	return strings.TrimPrefix(href, "/")
}

func (r *Resolver) hasPage(rel string) bool {
	r.ensureIndex()
	return r.index[rel]
}

// ensureIndex lazily scans PublicDir for .mu files.
func (r *Resolver) ensureIndex() {
	if r.indexed {
		return
	}
	r.indexed = true
	r.index = make(map[string]bool)
	if r.PublicDir == "" {
		return
	}
	_ = filepath.WalkDir(r.PublicDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".mu") {
			return nil
		}
		rel, err := filepath.Rel(r.PublicDir, p)
		if err != nil {
			return nil
		}
		r.index[filepath.ToSlash(rel)] = true
		return nil
	})
}

// Refresh clears the cached page index so the next lookup rescans.
func (r *Resolver) Refresh() {
	r.indexed = false
	r.index = nil
}

// contentDirOf maps a page path to the directory its Markdown source lives
// in, for resolving relative .md links.
func contentDirOf(fromPage string) string {
	if fromPage == "" || fromPage == "index" {
		return ""
	}
	if strings.HasSuffix(fromPage, "/index") {
		return strings.TrimSuffix(fromPage, "/index")
	}
	dir := path.Dir(fromPage)
	if dir == "." {
		return ""
	}
	return dir
}

// outputDirOf maps a page path to the directory used for relative links in
// the built output.
func outputDirOf(fromPage string) string {
	if fromPage == "index" {
		return ""
	}
	if strings.HasSuffix(fromPage, "/index") {
		return strings.TrimSuffix(fromPage, "/index")
	}
	dir := path.Dir(fromPage)
	if dir == "." {
		return ""
	}
	return dir
}

func isMarkdownPath(href string) bool {
	lower := strings.ToLower(href)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

// isMicronPath reports whether href points at a built Micron page. The .mu
// suffix marks page output, not a static file.
func isMicronPath(href string) bool {
	return strings.HasSuffix(strings.ToLower(href), ".mu")
}

func hasFileExtension(href string) bool {
	base := path.Base(href)
	return strings.LastIndexByte(base, '.') > 0
}
