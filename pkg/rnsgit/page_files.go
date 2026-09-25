// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// serveArtifact streams a release artifact for published releases only.
// Artifact names are restricted to basenames and tags resolve through the
// published-only list, matching the reference gate. The reference has a
// no-op guard at the artifact lookup that lets a missing file slip through.
// here a missing artifact returns no response.
func (n *Node) serveArtifact(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return nil
	}
	tag := vars["t"]
	if decoded, err := url.QueryUnescape(tag); err == nil {
		tag = decoded
	}
	if tag == "latest" {
		if _, latest := releasesListData(repoPath + ".releases"); latest != "" {
			tag = latest
		}
	}
	artifact := vars["a"]
	if decoded, err := url.QueryUnescape(artifact); err == nil {
		artifact = decoded
	}
	if tag == "" || artifact == "" || strings.Contains(artifact, "/") ||
		strings.Contains(tag, "/") || strings.Contains(tag, "..") {
		return nil
	}
	releaseDir := filepath.Join(repoPath+".releases", tag)
	info := releaseData(releaseDir, tag)
	if info == nil || info["status"] != "published" {
		return nil
	}
	artPath := filepath.Join(releaseDir, "artifacts", artifact)
	st, err := os.Stat(artPath)
	if err != nil || !st.Mode().IsRegular() {
		return nil
	}
	// defense in depth: confirm the resolved path is inside the release dir
	clean := filepath.Clean(artPath)
	root := filepath.Clean(filepath.Join(releaseDir, "artifacts"))
	if !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		return nil
	}
	blob, err := os.ReadFile(clean) // #nosec G304 -- artifact path validated above
	if err != nil {
		return nil
	}
	n.releaseDownloadSucceeded(group, repo, remote)
	meta, _ := msgpack.Marshal(map[string]any{"name": []byte(artifact)})
	return link.FileResponse{Data: blob, MetadataPacked: meta, AutoCompress: true}
}

// serveDownload streams a raw blob as a file response. The reference pipes
// git show directly. This implementation reads the blob with the page git
// timeout, which bounds both runtime and memory through the subprocess cap.
func (n *Node) serveDownload(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return nil
	}
	resolved, _, ok := n.pageRef(repoPath, vars["ref"])
	if !ok {
		return nil
	}
	filePath := vars["path"]
	if decoded, err := url.QueryUnescape(filePath); err == nil {
		filePath = decoded
	}
	filePath = strings.Trim(filePath, "/")
	if filePath == "" || strings.Contains(filePath, "..") {
		return nil
	}
	blob, err := n.git.ShowBlob(repoPath, resolved, filePath)
	if err != nil {
		return nil
	}
	n.downloadSucceeded(group, repo, remote)
	name := path.Base(filePath)
	meta, _ := msgpack.Marshal(map[string]any{"name": []byte(name)})
	return link.FileResponse{Data: blob, MetadataPacked: meta, AutoCompress: true}
}

// serveWorkDocDownload returns a work document as a packed [name, content]
// pair, matching the reference non-resource response shape.
func (n *Node) serveWorkDocDownload(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return nil
	}
	workPath := repoPath + ".work"
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		return nil
	}
	_, docDir, ok := workDocDir(workPath, id, workScopes)
	if !ok {
		return nil
	}
	hash := remotePageHash(remote)
	if !n.accessTable().ResolveDoc(group, repo, id, hash, permRead) {
		return nil
	}
	doc, err := loadWorkDoc(filepath.Join(docDir, "root"))
	if err != nil {
		return nil
	}
	meta := docMeta(doc)
	title := metaString(meta, "title", "document")
	name := sanitizeFilename(title) + "." + workDocExt(metaString(meta, "format", "markdown"))
	return []any{[]byte(name), []byte(reqStringVal(doc["content"]))}
}

// sanitizeFilename strips path and markup characters from a download name.
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '/' || r == '\\' || r == 0:
			b.WriteRune('_')
		case r < 32:
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "document"
	}
	return truncate(out, 120)
}

func workDocExt(format string) string {
	if format == "micron" {
		return "mu"
	}
	return "md"
}
