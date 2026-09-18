// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"strings"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

// serveRefsPage renders branches and tags with a type filter.
func (n *Node) serveRefsPage(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return n.renderTemplate(string(errBody), "", "refs", st)
	}
	filter := vars["type"]
	if filter != "heads" && filter != "tags" {
		filter = ""
	}
	icons := n.icons()

	base := map[string]string{"g": group, "r": repo}
	nav := n.repoNav(group, repo) + " / " + mLink("Refs", pagePathRefs, base)

	body := func(b *strings.Builder) {
		var filters []string
		filters = append(filters, mLinkR("All", pagePathRefs, base))
		filters = append(filters, mLinkR("Branches only", pagePathRefs, mergeFields(base, "type", "heads")))
		filters = append(filters, mLinkR("Tags only", pagePathRefs, mergeFields(base, "type", "tags")))
		b.WriteString(strings.Join(filters, "  "+icons.Sep+"  ") + "\n\n")

		refs, err := n.git.ForEachRef(repoPath)
		if err != nil {
			b.WriteString("Could not list refs\n")
			return
		}
		head := n.git.SymbolicRef(repoPath, "HEAD")

		var branches, tags []RefInfo
		for _, r := range refs {
			switch {
			case strings.HasPrefix(r.RefName, "refs/heads/"):
				branches = append(branches, r)
			case strings.HasPrefix(r.RefName, "refs/tags/"):
				tags = append(tags, r)
			}
		}

		if filter == "" || filter == "heads" {
			b.WriteString(mHeading(4) + " Branches\n\n")
			if len(branches) == 0 {
				b.WriteString("  No branches\n")
			}
			for _, r := range branches {
				def := ""
				if r.RefName == head {
					def = " " + mDim("(default)")
				}
				fields := mergeFields(base, "ref", r.RefName)
				b.WriteString("`a  " + icons.Branch + " " +
					mLink(r.Short, pagePathTree, fields) + def +
					"  " + mLinkR("[commits]", pagePathCommits, fields) +
					"  " + mDim(mEscape(truncate(r.Subject, 80))) + "`a\n")
			}
			b.WriteString("\n")
		}
		if filter == "" || filter == "tags" {
			b.WriteString(mHeading(4) + " Tags\n\n")
			if len(tags) == 0 {
				b.WriteString("  No tags\n")
			}
			for i := len(tags) - 1; i >= 0; i-- {
				r := tags[i]
				annotated := ""
				if r.ObjectType == "tag" {
					annotated = " " + mDim("(annotated)")
				}
				msg := r.Subject
				if r.ObjectType == "tag" && r.TagSubject != "" {
					msg = r.TagSubject
				}
				fields := mergeFields(base, "ref", r.RefName)
				b.WriteString("`a  " + icons.Tag + " " +
					mLink(r.Short, pagePathTree, fields) + annotated +
					"  " + mLinkR("[commits]", pagePathCommits, fields) +
					"  " + mDim(mEscape(truncate(msg, 512))) + "`a\n")
			}
		}
	}
	return n.renderPage("refs", nav, st, body)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
