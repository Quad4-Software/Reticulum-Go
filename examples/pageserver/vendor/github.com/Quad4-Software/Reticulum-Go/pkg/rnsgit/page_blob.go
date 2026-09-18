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

// renderableExts are blob extensions that can render formatted, matching the
// reference RENDERABLE_EXTS set.
var renderableExts = map[string]bool{".md": true, ".mu": true}

// serveBlobPage renders a single file: rendered micron/markdown when
// applicable, syntax-highlighted source otherwise, with raw toggles.
func (n *Node) serveBlobPage(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return n.renderTemplate(string(errBody), "", "blob", st)
	}
	resolved, displayRef, ok := n.pageRef(repoPath, vars["ref"])
	if !ok {
		return n.renderTemplate(pageError("Unknown ref"), "", "blob", st)
	}
	filePath := vars["path"]
	if decoded, err := url.QueryUnescape(filePath); err == nil {
		filePath = decoded
	}
	filePath = strings.Trim(filePath, "/")
	if filePath == "" || strings.Contains(filePath, "..") {
		return n.renderTemplate(pageError("Invalid path"), "", "blob", st)
	}

	info, err := n.git.BlobInfo(repoPath, resolved, filePath)
	if err != nil || !info.Exists {
		return n.renderTemplate(pageError("File not found"), "", "blob", st)
	}
	if info.IsTree {
		vars["path"] = filePath
		return n.serveTreePage("", repackVars(vars), nil, nil, remote, 0)
	}

	ext := strings.ToLower(path.Ext(filePath))
	renderable := renderableExts[ext]
	render := vars["render"] == "y" || (renderable && vars["raw"] != "y")

	fields := map[string]string{"g": group, "r": repo, "ref": displayRef, "path": filePath}
	nav := n.repoNav(group, repo) + " / " + mLink("Files", pagePathTree, fields)

	body := func(b *strings.Builder) {
		header := mHeading(2) + " " + mEscape(filePath) + " " +
			mDim(displayRef+" ("+resolved[:8]+")")
		kind := "Text"
		if info.Binary {
			kind = "Binary"
		}
		header += "\n" + mDim(kind+", "+prettySize(float64(info.Size)))
		if info.Symlink != "" {
			header += "  " + mDim("-> "+info.Symlink)
		}
		b.WriteString(header + "\n\n")

		var actions []string
		actions = append(actions, mLinkR("Download", filePathDownload, fields))
		if renderable {
			if render {
				actions = append(actions, mLinkR("View raw", pagePathBlob, mergeFields(fields, "raw", "y")))
			} else {
				actions = append(actions, mLinkR("View rendered", pagePathBlob, mergeFields(fields, "render", "y")))
			}
		}
		b.WriteString(strings.Join(actions, "  "+n.icons().Sep+"  ") + "\n\n")
		b.WriteString(mDivider() + "\n\n")

		switch {
		case info.Symlink != "":
			b.WriteString("Symbolic link to " + mEscape(info.Symlink) + "\n")
		case info.Binary:
			b.WriteString("Binary file, " + prettySize(float64(info.Size)) + ". It cannot be displayed.\n")
		case info.Size > blobSizeLimit:
			b.WriteString("This file is " + prettySize(float64(info.Size)) +
				", which exceeds the display limit of " + prettySize(blobSizeLimit) + ".\n")
		default:
			content, err := n.git.ShowBlob(repoPath, resolved, filePath)
			if err != nil {
				b.WriteString("Could not read file\n")
				return
			}
			text := string(content)
			if render && ext == ".mu" {
				b.WriteString(text)
			} else if render && ext == ".md" {
				dir := path.Dir(filePath)
				if dir == "." {
					dir = ""
				}
				scope := ":" + pagePathBlob + "`g=" + mFieldValue(group) +
					"|r=" + mFieldValue(repo) + "|ref=" + mFieldValue(displayRef) + "|path=" + mFieldValue(dir)
				if dir != "" {
					scope += "/"
				}
				b.WriteString(n.mdc.formatBlock(text, scope))
			} else if n.cfg.SyntaxHighlight {
				b.WriteString(n.hl.highlight(text, path.Base(filePath), ""))
			} else {
				b.WriteString(plainLiteral(text))
			}
		}
	}
	return n.renderPage("blob", nav, st, body)
}

// repackVars encodes a vars map back into a request dict so a blob handler
// can delegate to the tree handler when the path is a directory.
func repackVars(vars map[string]string) []byte {
	out := map[any]any{}
	for k, v := range vars {
		out["var_"+k] = v
	}
	b, _ := EncodeMixedRequest(out)
	return b
}

// plainLiteral wraps text in a micron literal block, escaping backticks.
func plainLiteral(content string) string {
	var b strings.Builder
	b.WriteString("`=\n")
	for line := range strings.Lines(content) {
		b.WriteString(mEscape(strings.TrimSuffix(line, "\n")))
		b.WriteString("\n")
	}
	b.WriteString("`=")
	return b.String()
}
