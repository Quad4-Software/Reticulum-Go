// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

// Page node constants. Pages are served on a dedicated nomadnetwork.node
// destination, the same destination aspect NomadNet clients browse, which is
// distinct from the git.repositories protocol destination.
const (
	pageAppName   = "nomadnetwork"
	pageAppAspect = "node"

	pagePathIndex    = "/page/index.mu"
	pagePathGroup    = "/page/group.mu"
	pagePathRepo     = "/page/repo.mu"
	pagePathTree     = "/page/tree.mu"
	pagePathBlob     = "/page/blob.mu"
	pagePathCommits  = "/page/commits.mu"
	pagePathCommit   = "/page/commit.mu"
	pagePathRefs     = "/page/refs.mu"
	pagePathStats    = "/page/stats.mu"
	pagePathReleases = "/page/releases.mu"
	pagePathRelease  = "/page/release.mu"
	pagePathWork     = "/page/work.mu"
	pagePathWorkDoc  = "/page/work_doc.mu"

	filePathArtifact = "/file/artifact"
	filePathDownload = "/file/download"
	filePathWorkDoc  = "/file/workdoc"

	pagePathMedia = "/media"

	// Display limits matching the reference page server.
	blobSizeLimit      = 256 * 1024
	treeEntriesPerPage = 1000
)

// pageRoutes maps request paths to handlers on the page destination.
func (n *Node) pageRoutes() []struct {
	path string
	fn   destination.ResponseGeneratorFunc
} {
	return []struct {
		path string
		fn   destination.ResponseGeneratorFunc
	}{
		{pagePathIndex, n.serveFrontPage},
		{pagePathGroup, n.serveGroupPage},
		{pagePathRepo, n.serveRepoPage},
		{pagePathTree, n.serveTreePage},
		{pagePathBlob, n.serveBlobPage},
		{pagePathCommits, n.serveCommitsPage},
		{pagePathCommit, n.serveCommitPage},
		{pagePathRefs, n.serveRefsPage},
		{pagePathStats, n.serveStatsPage},
		{pagePathReleases, n.serveReleasesPage},
		{pagePathRelease, n.serveReleasePage},
		{pagePathWork, n.serveWorkPage},
		{pagePathWorkDoc, n.serveWorkDocPage},
		{filePathArtifact, n.serveArtifact},
		{filePathDownload, n.serveDownload},
		{filePathWorkDoc, n.serveWorkDocDownload},
		{pagePathMedia, n.serveMediaRequest},
	}
}

// startPageNode creates the nomadnetwork.node destination and registers all
// page and file routes. Called from Start when serve_nomadnet is enabled.
func (n *Node) startPageNode() error {
	if n.n == nil {
		return fmt.Errorf("transport not ready")
	}
	dest, err := destination.New(n.identity, destination.In, destination.Single,
		pageAppName, n.n.Transport(), pageAppAspect)
	if err != nil {
		return fmt.Errorf("page destination: %w", err)
	}
	dest.SetDefaultAppData([]byte(n.cfg.NodeName))
	for _, r := range n.pageRoutes() {
		_ = dest.RegisterRequestHandlerAny(r.path, n.wrapPageHandler(r.path, r.fn),
			byte(destination.AllowAll), nil)
	}
	n.pageDest = dest
	if n.cfg.RecordStats {
		n.stats = newStatsStore(n.cfg.ConfigDir + "/stats")
		n.stats.run()
	}
	n.logf(debug.DebugWarning, "Git Nomad Network Node listening",
		"destination", hex.EncodeToString(dest.GetHash()))
	return nil
}

// announcePages announces the page destination with node name app data,
// matching the Python page node announce behavior.
func (n *Node) announcePages() {
	if n.pageDest != nil {
		_ = n.pageDest.Announce(false, nil, nil)
	}
}

// wrapPageHandler adds request logging and uniform panic containment to page
// handlers so a bad request cannot take down the destination.
func (n *Node) wrapPageHandler(path string, fn destination.ResponseGeneratorFunc) destination.ResponseGeneratorFunc {
	return func(p string, data []byte, reqID, linkID []byte, remote *identity.Identity, at int64) (out any) {
		defer func() {
			if r := recover(); r != nil {
				n.logf(debug.DebugError, "Page handler panic",
					"path", path, "panic", fmt.Sprint(r))
				out = n.renderTemplate(pageError("Internal error while rendering page"), "", "base", time.Now())
			}
		}()
		return fn(p, data, reqID, linkID, remote, at)
	}
}

// pageVars extracts the var_* parameters a NomadNet client sends for links
// with fields, matching the Browser var_ key convention.
func pageVars(data []byte) map[string]string {
	vars := map[string]string{}
	req, err := DecodeRequest(data)
	if err != nil {
		return vars
	}
	for k, v := range req {
		ks, ok := k.(string)
		if !ok {
			if kb, ok2 := k.([]byte); ok2 {
				ks = string(kb)
				ok = true
			}
		}
		if !ok || !strings.HasPrefix(ks, "var_") {
			continue
		}
		vars[strings.TrimPrefix(ks, "var_")] = reqStringVal(v)
	}
	return vars
}

func varInt(vars map[string]string, key string, def int) int {
	if v, ok := vars[key]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// accessibleRepo resolves and permission-checks group/repo for a page
// request. It returns the on-disk repo path or an error page body.
func (n *Node) accessibleRepo(vars map[string]string, remote *identity.Identity) (group, repo, repoPath string, errBody []byte) {
	group = vars["g"]
	repo = vars["r"]
	if group == "" || repo == "" {
		return "", "", "", []byte(pageError("No repository specified"))
	}
	rp, ok := n.repoPath(group, repo)
	if !ok {
		return "", "", "", []byte(pageError("Repository not found"))
	}
	hash := remotePageHash(remote)
	if !n.accessTable().Resolve(group, repo, hash, permRead) {
		return "", "", "", []byte(pageError("Repository not found"))
	}
	return group, repo, rp, nil
}

// pageRef resolves the ref var, defaulting to HEAD, and validates it through
// rev-parse so arbitrary strings cannot reach git argument positions.
func (n *Node) pageRef(repoPath, refVar string) (resolved, display string, ok bool) {
	ref := strings.TrimSpace(refVar)
	if ref == "" {
		ref = "HEAD"
	}
	out, err := n.git.RevParseVerify(repoPath, ref)
	if err != nil {
		return "", ref, false
	}
	sha := SanSHA(strings.TrimSpace(out))
	if sha == "" {
		return "", ref, false
	}
	return sha, ref, true
}

// pageNoIdent renders the no-identity page for blocked unidentified peers,
// matching the null_ident handling of the reference node.
func (n *Node) pageNoIdent() []byte {
	if tmpl := n.getTemplate("no_ident"); tmpl != "" {
		return []byte(tmpl)
	}
	return []byte(noIdentTemplate)
}
