// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

// serveReleasesPage lists published releases, matching the reference
// releases page. The reference renders the empty case through the repo
// template; here the releases template is used consistently.
func (n *Node) serveReleasesPage(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return n.renderTemplate(string(errBody), "", "releases", st)
	}
	icons := n.icons()
	nav := n.repoNav(group, repo) + " / " + mLink("Releases", pagePathReleases,
		map[string]string{"g": group, "r": repo})

	releases, latest := releasesListData(repoPath + ".releases")

	body := func(b *strings.Builder) {
		b.WriteString(mHeading(3) + " Releases\n\n")
		shown := 0
		for _, info := range releases {
			if info["status"] != "published" {
				continue
			}
			shown++
			tag := reqStringVal(info["tag"])
			b.WriteString("`a  " + icons.Package + " " +
				mLink(tag, pagePathRelease, map[string]string{
					"g": group, "r": repo, "t": tag,
				}))
			if tag == latest {
				b.WriteString(" " + mFg("Latest", "0a0"))
			}
			b.WriteString("  " + mDim(prettyTimeAgo(int64(metaNum(info["created"])))))
			if n, ok := info["artifacts"].(int); ok && n > 0 {
				b.WriteString("  " + mDim(fmtInt(n)+" artifacts"))
			}
			b.WriteString("`a\n")
			if preview, ok := info["preview"].(string); ok && preview != "" {
				preview = truncate(preview, 2048)
				switch info["format"] {
				case "markdown":
					b.WriteString(indent(n.mdc.formatBlock(preview, ""), "    "))
				case "micron":
					b.WriteString(indent(preview, "    "))
				default:
					b.WriteString(indent(mEscape(preview), "    "))
				}
				b.WriteString("\n")
			}
		}
		if shown == 0 {
			b.WriteString("No published releases\n")
		}
	}
	return n.renderPage("releases", nav, st, body)
}

// serveReleasePage renders a single release with notes, artifacts and the
// thanks counter.
func (n *Node) serveReleasePage(_ string, data []byte, _ []byte, linkID []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return n.renderTemplate(string(errBody), "", "release", st)
	}
	releasesPath := repoPath + ".releases"
	tag := vars["t"]
	if tag == "" {
		tag = "latest"
	}
	if tag == "latest" {
		if _, latest := releasesListData(releasesPath); latest != "" {
			tag = latest
		} else {
			return n.renderTemplate(pageError("No releases published"), "", "release", st)
		}
	}
	if strings.Contains(tag, "/") || strings.Contains(tag, "..") {
		return n.renderTemplate(pageError("Invalid release"), "", "release", st)
	}
	releaseDir := filepath.Join(releasesPath, tag)
	info := releaseData(releaseDir, tag)
	if info == nil || info["status"] != "published" {
		return n.renderTemplate(pageError("Release not found"), "", "release", st)
	}
	icons := n.icons()
	nav := n.repoNav(group, repo) + " / " +
		mLink("Releases", pagePathReleases, map[string]string{"g": group, "r": repo}) +
		" / " + mEscape(tag)

	thanks := n.releaseThanks(releaseDir, vars["thanks"] == "y", linkID)

	body := func(b *strings.Builder) {
		b.WriteString(mHeading(2) + " " + mEscape(tag) + "\n")
		b.WriteString(mDim(prettyDate(int64(metaNum(info["created"])))) + "\n\n")
		b.WriteString(mLinkR(icons.Heart+" Thanks("+fmtInt64(thanks)+")", pagePathRelease,
			map[string]string{"g": group, "r": repo, "t": tag, "thanks": "y"}) + "\n\n")

		if notes, ok := info["notes"].(string); ok && notes != "" {
			switch info["notes_format"] {
			case "micron":
				b.WriteString(notes + "\n")
			case "text":
				b.WriteString(mEscape(notes) + "\n")
			default:
				b.WriteString(n.mdc.formatBlock(notes, "") + "\n")
			}
		}
		if arts, ok := info["artifacts"].([]map[string]any); ok && len(arts) > 0 {
			b.WriteString("\n" + mHeading(4) + " Artifacts\n\n")
			for _, a := range arts {
				name := reqStringVal(a["name"])
				size := toInt64(a["size"])
				b.WriteString("`a  " + icons.Package + " " +
					mLink(name, filePathArtifact, map[string]string{
						"g": group, "r": repo, "t": tag, "a": name,
					}) + "  " + mDim(prettySize(float64(size))) + "`a\n")
			}
		}
	}
	return n.renderPage("release", nav, st, body)
}

// metaNum reads a numeric META value regardless of decode type.
func metaNum(v any) float64 {
	switch x := v.(type) {
	case int64:
		return float64(x)
	case int:
		return float64(x)
	case float64:
		return x
	case string:
		f, _ := parseUnixFloat(x)
		return f
	default:
		return 0
	}
}

func parseUnixFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func indent(s, prefix string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString(prefix)
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
