package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// navEntry is one document in the navigation order.
type navEntry struct {
	Slug  string
	Label string
}

// navFile mirrors site/data/docsnav.toml.
type navFile struct {
	Groups []struct {
		Name  string `toml:"name"`
		Pages []struct {
			Slug  string `toml:"slug"`
			Label string `toml:"label"`
		} `toml:"pages"`
	} `toml:"groups"`
}

// loadNav reads the docsnav data file and returns the flat, ordered entry
// list plus the group names for the index document.
func loadNav(path string) ([]navEntry, [][2]string, error) {
	var nf navFile
	if _, err := toml.DecodeFile(path, &nf); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var flat []navEntry
	var groups [][2]string
	for _, g := range nf.Groups {
		for _, p := range g.Pages {
			flat = append(flat, navEntry{Slug: p.Slug, Label: p.Label})
			groups = append(groups, [2]string{g.Name, p.Slug})
		}
	}
	if len(flat) == 0 {
		return nil, nil, fmt.Errorf("%s lists no pages", path)
	}
	return flat, groups, nil
}

var changelogVersionRE = regexp.MustCompile(`^##\s+v?(\d+\.\d+\.\d+)`)

// docVersion returns the version of the top released heading in the
// changelog. Headings marked unreleased are skipped. When no released
// heading exists it returns "unreleased".
func docVersion(changelog string) string {
	for _, line := range strings.Split(changelog, "\n") {
		if !strings.HasPrefix(line, "##") {
			continue
		}
		if strings.Contains(strings.ToLower(line), "unreleased") {
			continue
		}
		if m := changelogVersionRE.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return "unreleased"
}

var mdHeadingRE = regexp.MustCompile(`(?m)^#[ \t]+(\S.*?)[ \t#]*$`)

// docTitle extracts the first level one heading from Markdown source,
// falling back to the slug when none exists.
func docTitle(slug, source string) string {
	if m := mdHeadingRE.FindStringSubmatch(source); m != nil {
		return strings.TrimSpace(m[1])
	}
	return slug
}

// rewriteDocLinks rewrites Markdown link destinations outside fenced code
// blocks and inline code spans. mapFn receives the raw destination and
// returns the replacement. A destination that starts with "<" and ends with
// ">" keeps its angle brackets.
func rewriteDocLinks(src string, mapFn func(dest string) string) string {
	lines := strings.Split(src, "\n")
	fence := false
	linkRE := regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)`)
	codeRE := regexp.MustCompile("`[^`]*`")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		// Mask inline code spans so their contents are not rewritten.
		var spans []string
		masked := codeRE.ReplaceAllStringFunc(line, func(s string) string {
			spans = append(spans, s)
			return fmt.Sprintf("\x00%d\x00", len(spans)-1)
		})
		masked = linkRE.ReplaceAllStringFunc(masked, func(m string) string {
			open := strings.LastIndex(m, "(")
			dest := m[open+1:]
			return m[:open+1] + mapFn(dest)
		})
		maskRE := regexp.MustCompile("\x00(\\d+)\x00")
		line = maskRE.ReplaceAllStringFunc(masked, func(m string) string {
			var n int
			if _, err := fmt.Sscanf(m, "\x00%d\x00", &n); err == nil && n < len(spans) {
				return spans[n]
			}
			return m
		})
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
