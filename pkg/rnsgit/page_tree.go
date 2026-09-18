// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

// serveTreePage renders the file browser for a ref/path with pagination.
func (n *Node) serveTreePage(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return n.renderTemplate(string(errBody), "", "tree", st)
	}
	resolved, displayRef, ok := n.pageRef(repoPath, vars["ref"])
	if !ok {
		return n.renderTemplate(pageError("Unknown ref"), "", "tree", st)
	}
	dirPath := vars["path"]
	if decoded, err := url.QueryUnescape(dirPath); err == nil {
		dirPath = decoded
	}
	dirPath = strings.Trim(dirPath, "/")
	page := varInt(vars, "page", 1)
	if page < 1 {
		page = 1
	}
	icons := n.icons()

	nav := n.repoNav(group, repo) + " / " + mLink("Files", pagePathTree,
		map[string]string{"g": group, "r": repo})
	if dirPath != "" {
		nav += " / " + mEscape(dirPath)
	}

	body := func(b *strings.Builder) {
		b.WriteString(mHeading(3) + " " + mEscape(repo) + " " +
			mDim(displayRef+" ("+resolved[:8]+")") + "\n\n")

		entries, err := n.git.LsTree(repoPath, resolved, dirPath)
		if err != nil {
			b.WriteString("Cannot list this path\n")
			return
		}
		base := map[string]string{"g": group, "r": repo, "ref": displayRef}
		if dirPath != "" {
			parent := path.Dir(dirPath)
			if parent == "." {
				parent = ""
			}
			up := map[string]string{"g": group, "r": repo, "ref": displayRef}
			if parent != "" {
				up["path"] = parent
			}
			b.WriteString(mLink("..", pagePathTree, up) + "\n\n")
		}
		total := len(entries)
		pages := (total + treeEntriesPerPage - 1) / treeEntriesPerPage
		if pages < 1 {
			pages = 1
		}
		if page > pages {
			page = pages
		}
		start := (page - 1) * treeEntriesPerPage
		end := start + treeEntriesPerPage
		if end > total {
			end = total
		}
		for _, e := range entries[start:end] {
			entryPath := e.Name
			if dirPath != "" {
				entryPath = dirPath + "/" + e.Name
			}
			switch e.Type {
			case "tree":
				f := mergeFields(base, "path", entryPath)
				b.WriteString("`a  " + icons.Folder + " " +
					mLink(e.Name+"/", pagePathTree, f) + "`a\n")
			case "commit":
				b.WriteString("`a  ⧉ " + mEscape(e.Name) + " " +
					mDim("(submodule)") + "`a\n")
			case "link":
				b.WriteString("`a  ↳ " + mEscape(e.Name) + " " +
					mDim("-> "+e.Link) + "`a\n")
			default:
				f := mergeFields(base, "path", entryPath)
				b.WriteString("`a  " + icons.File + " " +
					mLink(e.Name, pagePathBlob, f) + "  " +
					mDim(prettySize(float64(e.Size))) + "`a\n")
			}
		}
		if total == 0 {
			b.WriteString("Empty\n")
		}
		if pages > 1 {
			b.WriteString("\n")
			pf := mergeFields(base, "path", dirPath)
			if page > 1 {
				pf["page"] = fmtInt(page - 1)
				b.WriteString(mLinkR("« Previous", pagePathTree, pf))
				b.WriteString("   ")
			}
			b.WriteString(mDim("Page " + fmtInt(page) + " of " + fmtInt(pages)))
			if page < pages {
				pf["page"] = fmtInt(page + 1)
				b.WriteString("   " + mLinkR("Next »", pagePathTree, pf))
			}
			b.WriteString("\n")
		}
	}
	return n.renderPage("tree", nav, st, body)
}

// mergeFields copies a field map and sets one key.
func mergeFields(base map[string]string, k, v string) map[string]string {
	out := make(map[string]string, len(base)+1)
	for key, val := range base {
		out[key] = val
	}
	if v != "" {
		out[k] = v
	}
	return out
}

// repoNav renders the shared "Node / group / repo" breadcrumb.
func (n *Node) repoNav(group, repo string) string {
	return mLinkStrong("Node", pagePathIndex, nil) + " / " +
		mLink(group, pagePathGroup, map[string]string{"g": group}) + " / " +
		mLink(repo, pagePathRepo, map[string]string{"g": group, "r": repo})
}
