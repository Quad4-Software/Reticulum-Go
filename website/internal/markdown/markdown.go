// Package markdown converts Markdown to Micron markup.
//
// The converter targets the NomadNet Micron dialect: headings use > prefixes,
// emphasis uses backtick commands such as `! for bold, and literal blocks use
// `= fences. It is built on goldmark, the same parser Hugo uses, with the GFM
// and footnote extensions enabled.
package markdown

import "strings"

// DefaultMaxWidth is the fallback table width budget in characters.
const DefaultMaxWidth = 100

// ImagePolicy controls how Markdown images are emitted.
type ImagePolicy int

const (
	// ImageLink emits an image as a Micron link with image fields, like
	// `[alt`url`img=1]. Micron clients render it as an image when the URL
	// normalizes to a media path or a /file/ WebP path. Other destinations
	// stay plain links.
	ImageLink ImagePolicy = iota
	// ImageAlt emits only the alt text.
	ImageAlt
	// ImageSkip drops the image entirely.
	ImageSkip
)

// LinkStyle controls how Micron links are wrapped.
type LinkStyle int

const (
	// LinkPlain emits a bare `link.
	LinkPlain LinkStyle = iota
	// LinkUnderline wraps links in `_.
	LinkUnderline
	// LinkBold wraps links in `!.
	LinkBold
	// LinkBoth wraps links in `_! and closes with `!`_. This matches the
	// NomadNet MarkdownToMicron defaults.
	LinkBoth
	// LinkColor wraps links in a `F or `FT color command. Requires LinkColor.
	LinkColor
)

// Options configures the Markdown to Micron conversion.
type Options struct {
	// MaxWidth is the character budget for table layout. Zero uses
	// DefaultMaxWidth.
	MaxWidth int

	// RewriteLink maps a link destination to the URL written into the Micron
	// output. It is called for every link. A nil func leaves destinations
	// unchanged.
	RewriteLink func(url string) string

	// RewriteImageLink maps an image destination. When nil, RewriteLink is
	// used. Set this to route images to a file or media scope such as
	// ":/file/".
	RewriteImageLink func(url string) string

	// ImagePolicy selects image handling. Zero value is ImageLink.
	ImagePolicy ImagePolicy

	// LinkStyle selects link wrapping. Zero value is LinkPlain.
	LinkStyle LinkStyle

	// LinkColor is a three or six digit hex color used when LinkStyle is
	// LinkColor. Invalid values fall back to LinkPlain.
	LinkColor string

	// TableAlign wraps the rendered table in a Micron block alignment
	// command. Valid values are "", "l", "c", and "r". "left", "center",
	// and "right" are also accepted.
	TableAlign string
}

// linkScopePrefix is the default scope for relative links, matching the
// NomadNet MarkdownToMicron convention.
const linkScopePrefix = ":/page/"

// DefaultRewriteLink returns a RewriteLink that prefixes relative
// destinations with scope and moves URL fragments into Micron anchor fields.
// nodeHash, when non-empty, prefixes scoped links as "<hash>:<scope>...".
func DefaultRewriteLink(scope, nodeHash string) func(string) string {
	if scope == "" {
		scope = linkScopePrefix
	}
	return func(url string) string {
		return rewriteLink(url, scope, nodeHash)
	}
}

// rewriteLink applies the NomadNet link rules. Destinations that carry a
// node scope (":/") are left alone. External URLs with a scheme pass through
// unchanged and drop any fragment, matching the reference converter. Plain
// destinations are treated as site relative and prefixed with scope, and a
// fragment becomes a "|anchor=" suffix.
func rewriteLink(url, scope, nodeHash string) string {
	if strings.HasPrefix(url, "#") {
		return url
	}
	frag := ""
	if i := strings.IndexByte(url, '#'); i >= 0 {
		frag = url[i+1:]
		url = url[:i]
	}
	if strings.Contains(url, "://") || strings.HasPrefix(url, "mailto:") ||
		strings.HasPrefix(url, "tel:") {
		return url
	}
	if !strings.Contains(url, ":/") {
		if nodeHash != "" {
			scope = nodeHash + ":" + strings.TrimPrefix(scope, ":")
		}
		url = scope + strings.TrimPrefix(url, "/")
		url = strings.TrimSuffix(url, ".md")
		url = strings.TrimSuffix(url, ".markdown")
	}
	if frag != "" {
		url += "|anchor=" + frag
	}
	return url
}
