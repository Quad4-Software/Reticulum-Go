// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

// workDocCount counts numeric document directories in a scope.
func (n *Node) workDocCount(repoPath, scope string) int {
	workPath := repoPath + ".work"
	entries, err := os.ReadDir(filepath.Join(workPath, scope))
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err == nil {
			count++
		}
	}
	return count
}

// workListEntry is one row on the work documents page.
type workListEntry struct {
	ID       int
	Scope    string
	Title    string
	Created  float64
	Edited   float64
	Author   string
	Format   string
	Comments int
}

// workListScope gathers document rows for one scope honoring doc-level read
// permission, matching the reference work page listing.
func (n *Node) workListScope(workPath, group, repo, scope string, remote *identity.Identity) []workListEntry {
	dir := filepath.Join(workPath, scope)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	hash := remotePageHash(remote)
	var out []workListEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if !n.accessTable().ResolveDoc(group, repo, id, hash, permRead) {
			continue
		}
		doc, err := loadWorkDoc(filepath.Join(dir, e.Name(), "root"))
		if err != nil {
			continue
		}
		meta := docMeta(doc)
		comments := 0
		if files, ferr := os.ReadDir(filepath.Join(dir, e.Name())); ferr == nil {
			for _, f := range files {
				if f.IsDir() || f.Name() == "root" {
					continue
				}
				if _, err := strconv.Atoi(f.Name()); err == nil {
					comments++
				}
			}
		}
		out = append(out, workListEntry{
			ID:       id,
			Scope:    scope,
			Title:    truncate(metaString(meta, "title", "Untitled"), 92),
			Created:  metaFloat(meta, "created"),
			Edited:   metaFloat(meta, "edited"),
			Author:   metaAuthorHex(meta),
			Format:   metaString(meta, "format", "markdown"),
			Comments: comments,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out
}

// serveWorkPage lists work documents in the selected scope.
func (n *Node) serveWorkPage(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return n.renderTemplate(string(errBody), "", "work", st)
	}
	workPath := repoPath + ".work"
	scope := vars["scope"]
	if !workScopesValid(scope) {
		scope = "active"
	}
	icons := n.icons()
	base := map[string]string{"g": group, "r": repo}
	nav := n.repoNav(group, repo) + " / " + mLink("Work", pagePathWork, base)

	body := func(b *strings.Builder) {
		var scopes []string
		if scope == "all" {
			scopes = workScopes
		} else {
			scopes = []string{scope}
		}
		var filters []string
		for _, s := range append(append([]string{}, workScopes...), "all") {
			filters = append(filters, mLinkR(s, pagePathWork, mergeFields(base, "scope", s)))
		}
		b.WriteString(strings.Join(filters, "  "+icons.Sep+"  ") + "\n\n")

		total := 0
		for _, s := range scopes {
			entries := n.workListScope(workPath, group, repo, s, remote)
			if len(entries) == 0 {
				continue
			}
			total += len(entries)
			b.WriteString(mHeading(4) + " " + capitalize(s) + "\n\n")
			for _, e := range entries {
				b.WriteString("`a  " + icons.Work + " " +
					mLink(e.Title, pagePathWorkDoc, map[string]string{
						"g": group, "r": repo, "id": strconv.Itoa(e.ID), "scope": e.Scope,
					}) +
					"  " + mDim("#"+strconv.Itoa(e.ID)) +
					"  " + mDim(formatRelativeTime(int64(e.Created))) +
					"  " + mDim("by "+e.Author))
				if e.Comments > 0 {
					b.WriteString("  " + mDim(strconv.Itoa(e.Comments)+" updates"))
				}
				b.WriteString("`a\n")
			}
			b.WriteString("\n")
		}
		if total == 0 {
			b.WriteString("No work documents\n")
		}
	}
	return n.renderPage("work", nav, st, body)
}

// serveWorkDocPage renders a single work document with its updates. The
// reference resets an invalid scope to "active" while scope "all" probes in
// order; this implementation probes all scopes whenever the given scope does
// not contain the document.
func (n *Node) serveWorkDocPage(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return n.renderTemplate(string(errBody), "", "work_doc", st)
	}
	workPath := repoPath + ".work"
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		return n.renderTemplate(pageError("No document specified"), "", "work_doc", st)
	}
	scope, docDir, ok := workDocDir(workPath, id, workScopes)
	if !ok {
		return n.renderTemplate(pageError("Document not found"), "", "work_doc", st)
	}
	hash := remotePageHash(remote)
	if !n.accessTable().ResolveDoc(group, repo, id, hash, permRead) {
		return n.renderTemplate(pageError("Document not found"), "", "work_doc", st)
	}
	doc, err := loadWorkDoc(filepath.Join(docDir, "root"))
	if err != nil {
		return n.renderTemplate(pageError("Could not load document"), "", "work_doc", st)
	}
	meta := docMeta(doc)
	content := reqStringVal(doc["content"])
	base := map[string]string{"g": group, "r": repo}
	nav := n.repoNav(group, repo) + " / " + mLink("Work", pagePathWork, base) +
		" / " + mEscape(strconv.Itoa(id))

	sigOK := verifyWorkDocSignature(meta, content)
	author := metaAuthorHex(meta)

	body := func(b *strings.Builder) {
		b.WriteString(mHeading(2) + " " + mEscape(truncate(metaString(meta, "title", "Untitled"), 256)) + "\n\n")
		b.WriteString(mDim("#"+strconv.Itoa(id)+" in "+scope) + "\n")
		if author != "" {
			b.WriteString("Author: " + mEscape(author))
			if sigOK {
				b.WriteString(" " + mFg("signature valid", "0a0"))
			} else {
				b.WriteString(" " + mFg("signature invalid", "f00"))
			}
			b.WriteString("\n")
		}
		b.WriteString(mDim("Created "+prettyDate(int64(metaFloat(meta, "created")))+
			", edited "+formatRelativeTime(int64(metaFloat(meta, "edited")))) + "\n\n")
		b.WriteString(mLinkR("Download", filePathWorkDoc, map[string]string{
			"g": group, "r": repo, "id": strconv.Itoa(id), "scope": scope,
		}) + "\n\n")
		b.WriteString(mDivider() + "\n\n")

		if metaString(meta, "format", "markdown") == "micron" {
			b.WriteString(content + "\n")
		} else {
			b.WriteString(n.mdc.formatBlock(content, "") + "\n")
		}

		// updates: numeric sibling comment files
		files, _ := os.ReadDir(docDir)
		type comment struct {
			id  int
			doc map[string]any
		}
		var comments []comment
		for _, f := range files {
			if f.IsDir() || f.Name() == "root" {
				continue
			}
			cid, err := strconv.Atoi(f.Name())
			if err != nil {
				continue
			}
			if cdoc, err := loadWorkDoc(filepath.Join(docDir, f.Name())); err == nil {
				comments = append(comments, comment{cid, cdoc})
			}
		}
		if len(comments) > 0 {
			sort.Slice(comments, func(i, j int) bool { return comments[i].id < comments[j].id })
			b.WriteString("\n" + mDivider() + "\n\n" + mHeading(4) + " Updates\n\n")
			for _, c := range comments {
				cm := docMeta(c.doc)
				cauthor := metaAuthorHex(cm)
				csig := verifyWorkDocSignature(cm, reqStringVal(c.doc["content"]))
				b.WriteString("`a  " + mDim("#"+strconv.Itoa(c.id)+"  "+formatRelativeTime(int64(metaFloat(cm, "edited")))+"  by "+cauthor))
				if csig {
					b.WriteString(" " + mFg("signed", "0a0"))
				}
				b.WriteString("`a\n")
				ccontent := reqStringVal(c.doc["content"])
				if metaString(cm, "format", "markdown") == "micron" {
					b.WriteString(indent(ccontent, "    "))
				} else {
					b.WriteString(indent(n.mdc.formatBlock(ccontent, ""), "    "))
				}
				b.WriteString("\n")
			}
		}
	}
	return n.renderPage("work_doc", nav, st, body)
}

// verifyWorkDocSignature validates the meta signature over the document
// content, matching the reference signature check on work doc pages.
func verifyWorkDocSignature(meta map[string]any, content string) bool {
	sig := metaBytes(meta, "signature")
	pub := metaBytes(meta, "identity")
	if len(sig) != 64 || len(pub) == 0 {
		return false
	}
	id := identity.FromPublicKey(pub)
	if id == nil {
		return false
	}
	return id.Verify([]byte(content), sig)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
