// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// Page paths served by the nomadnet node, matching Python NomadNetworkNode.
const (
	pagePathIndex = "/page/index.mu"
	pagePathMedia = "/media"
)

// imageExts are media extensions eligible for WebP conversion, matching
// Python IMAGE_EXTS.
var imageExts = map[string]bool{
	".webp": true, ".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".tiff": true, ".tif": true, ".bmp": true,
}

// nullIdentHash is the identity hash of the all-zero public key used for
// unidentified peers, matching Python null_ident.
const nullIdentHash = "d7db22f63b453c23bb0688dde565b7c1"

// noIdentTemplate matches Python DEFAULT_NO_IDENT_TEMPLATE.
const noIdentTemplate = ">>No Identity\n\nThis page requires identification, and none was received.\n"

func (n *Node) startPageNode() error {
	if n.dest == nil {
		return fmt.Errorf("destination not ready")
	}
	_ = n.dest.RegisterRequestHandlerAny(pagePathIndex, func(_ string, _ []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
		return n.renderRepoIndex(remote)
	}, destination.AllowAll, nil)
	_ = n.dest.RegisterRequestHandlerAny(pagePathMedia, func(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
		return n.serveMedia(data, remote)
	}, destination.AllowAll, nil)
	return nil
}

// remotePageHash returns the identity hash used for page-level permission
// checks, substituting the null identity for unidentified peers like Python.
func remotePageHash(remote *identity.Identity) []byte {
	if remote == nil {
		h, _ := hex.DecodeString(nullIdentHash)
		return h
	}
	return remote.Hash()
}

func (n *Node) renderRepoIndex(remote *identity.Identity) []byte {
	hash := remotePageHash(remote)
	access := n.accessTable()
	if remote == nil && access.Blocked[nullIdentHash] {
		return []byte(noIdentTemplate)
	}
	var b strings.Builder
	b.WriteString(">Reticulum Git Node\n\n")
	b.WriteString(n.cfg.NodeName)
	b.WriteString("\n\nRepositories destination:\n`F")
	b.WriteString(n.ReposDestHash())
	b.WriteString("`\n\n")
	var groups []string
	for group := range access.Groups {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	for _, group := range groups {
		ga := access.Groups[group]
		var repos []string
		for name := range ga.Repositories {
			if access.Resolve(group, name, hash, permRead) {
				repos = append(repos, name)
			}
		}
		if len(repos) == 0 {
			continue
		}
		sort.Strings(repos)
		b.WriteString(group)
		b.WriteString(" (")
		b.WriteString(fmt.Sprintf("%d", len(repos)))
		b.WriteString(" repos)\n")
		for _, name := range repos {
			b.WriteString("  - ")
			b.WriteString(name)
			b.WriteString("\n")
		}
	}
	return []byte(b.String())
}

// serveMedia implements the Python serve_media handler. It serves repository
// blobs and optionally converts images to WebP.
func (n *Node) serveMedia(data []byte, remote *identity.Identity) any {
	req, err := DecodeRequest(data)
	if err != nil || req == nil {
		req = map[any]any{}
	}
	if _, ok := req["key"]; !ok {
		return nil
	}
	mediaPath := reqString(req, "path")
	if mediaPath == "" {
		return nil
	}
	comps := strings.Split(strings.TrimLeft(strings.TrimPrefix(mediaPath, "/media"), "/"), "/")
	if len(comps) < 4 {
		return nil
	}
	group := comps[0]
	repo := comps[1]
	ref := comps[2]
	filePath := strings.Join(comps[3:], "/")
	if decoded, err := url.QueryUnescape(filePath); err == nil {
		filePath = decoded
	}
	fileName := path.Base(filePath)

	hash := remotePageHash(remote)
	access := n.accessTable()
	repoPath, ok := n.repoPath(group, repo)
	if !ok || !access.Resolve(group, repo, hash, permRead) {
		return nil
	}
	resolved, err := n.git.RevParseVerify(repoPath, ref)
	if err != nil || SanSHA(resolved) == "" {
		return nil
	}
	resolved = SanSHA(resolved)
	if filePath == "" {
		return nil
	}
	if _, err := n.git.BlobSize(repoPath, resolved, filePath); err != nil {
		return nil
	}
	ext := strings.ToLower(path.Ext(filePath))
	if n.cfg.MediaConversion && imageExts[ext] && ext != ".webp" {
		if data, name, ok := n.spoolWebP(repoPath, resolved, filePath); ok {
			meta, _ := msgpack.Marshal(map[string]any{"name": []byte(name)})
			return link.FileResponse{Data: data, MetadataPacked: meta, AutoCompress: false}
		}
	}
	blob, err := n.git.BlobContent(repoPath, resolved, filePath)
	if err != nil {
		return nil
	}
	meta, _ := msgpack.Marshal(map[string]any{"name": []byte(fileName)})
	return link.FileResponse{Data: blob, MetadataPacked: meta, AutoCompress: false}
}
