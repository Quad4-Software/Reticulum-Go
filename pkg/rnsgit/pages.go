// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/hex"
	"net/url"
	"path"
	"strings"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// imageExts are media extensions eligible for WebP conversion.
var imageExts = map[string]bool{
	".webp": true, ".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".tiff": true, ".tif": true, ".bmp": true,
}

// nullIdentHash is the identity hash of the all-zero public key used for
// unidentified peers, matching Python null_ident.
const nullIdentHash = "d7db22f63b453c23bb0688dde565b7c1"

// noIdentTemplate matches Python DEFAULT_NO_IDENT_TEMPLATE.
const noIdentTemplate = ">>No Identity\n\nThis page requires identification, and none was received.\n"

// remotePageHash returns the identity hash used for page-level permission
// checks, substituting the null identity for unidentified peers like Python.
func remotePageHash(remote *identity.Identity) []byte {
	if remote == nil {
		h, _ := hex.DecodeString(nullIdentHash)
		return h
	}
	return remote.Hash()
}

// serveMediaRequest wraps the media handler for the page destination.
func (n *Node) serveMediaRequest(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	return n.serveMedia(data, remote)
}

// serveMedia serves repository blobs to page clients and optionally converts
// images to WebP. This endpoint is a Reticulum-Go extension: the reference
// implementation exposes blobs only through /file/download. Media requests
// carry {"key": <token>, "path": "/media/group/repo/ref/path"}.
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
	n.downloadSucceeded(group, repo, remote)
	meta, _ := msgpack.Marshal(map[string]any{"name": []byte(fileName)})
	return link.FileResponse{Data: blob, MetadataPacked: meta, AutoCompress: false}
}
