// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
)

func (n *Node) handlePerms(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	req, err := DecodeRequest(data)
	if err != nil {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	op := fmt.Sprint(req["operation"])
	step := fmt.Sprint(req["step"])
	switch op {
	case "gperms":
		group := fmt.Sprint(req[IdxGroup])
		if group == "" {
			if rp, ok := RepoFromRequest(req); ok {
				group, _, _ = ParseRepoPath(rp)
			}
		}
		if group == "" {
			return StatusResponse(ResInvalidReq, "Invalid request")
		}
		if !n.access.Resolve(group, "", remote.Hash(), permAdmin) {
			return StatusResponse(ResDisallowed, "Not allowed")
		}
		ga, ok := n.access.Groups[group]
		if !ok {
			return StatusResponse(ResNotFound, "Not found")
		}
		allowedPath := filepath.Join(filepath.Dir(ga.Path), group+".allowed")
		if step == "get" {
			content, _ := ReadAllowedFile(allowedPath)
			return append([]byte{ResOK}, []byte(content)...)
		}
		if step == "set" {
			content := fmt.Sprint(req["content"])
			if err := WriteAllowedFile(allowedPath, content); err != nil {
				return StatusResponse(ResInvalidReq, "Invalid permissions")
			}
			_ = n.reloadAccess()
			return []byte{ResOK}
		}
	case "rperms":
		repoPath, ok := RepoFromRequest(req)
		if !ok {
			return StatusResponse(ResInvalidReq, "Invalid request")
		}
		group, repo, ok := ParseRepoPath(repoPath)
		if !ok {
			return StatusResponse(ResInvalidReq, "Invalid request")
		}
		if !n.remoteAllowed(remote, group, repo, permAdmin) {
			return StatusResponse(ResDisallowed, "Not allowed")
		}
		ga, ok := n.access.Groups[group]
		if !ok {
			return StatusResponse(ResNotFound, "Not found")
		}
		allowedPath := filepath.Join(ga.Path, repo+".allowed")
		if step == "get" {
			content, _ := ReadAllowedFile(allowedPath)
			return append([]byte{ResOK}, []byte(content)...)
		}
		if step == "set" {
			content := fmt.Sprint(req["content"])
			if err := WriteAllowedFile(allowedPath, content); err != nil {
				return StatusResponse(ResInvalidReq, "Invalid permissions")
			}
			_ = n.reloadAccess()
			return []byte{ResOK}
		}
	}
	return StatusResponse(ResInvalidReq, "Invalid request")
}

func (n *Node) handleRelease(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	req, err := DecodeRequest(data)
	if err != nil {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	repoPath, ok := RepoFromRequest(req)
	if !ok {
		return StatusResponse(ResInvalidReq, "No repository specified")
	}
	group, repo, ok := ParseRepoPath(repoPath)
	if !ok {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	op := fmt.Sprint(req["operation"])
	switch op {
	case "list":
		if !n.remoteAllowed(remote, group, repo, permRead) {
			return StatusResponse(ResNotFound, "Not found")
		}
		body, err := n.listReleases(group, repo)
		if err != nil {
			return StatusResponse(ResRemoteFail, "Could not list releases")
		}
		return append([]byte{ResOK}, body...)
	case "fetch":
		if !n.remoteAllowed(remote, group, repo, permRead) {
			return StatusResponse(ResNotFound, "Not found")
		}
		tag := fmt.Sprint(req["tag"])
		artifact := fmt.Sprint(req["artifact"])
		fileData, err := n.fetchReleaseArtifact(group, repo, tag, artifact)
		if err != nil {
			return StatusResponse(ResNotFound, "Not found")
		}
		return link.FileResponse{Data: fileData, MetadataPacked: OKMetadataPacked(), AutoCompress: true}
	case "create":
		if !n.remoteAllowed(remote, group, repo, permRelease) {
			return StatusResponse(ResDisallowed, "Not allowed")
		}
		tag := fmt.Sprint(req["tag"])
		notes := fmt.Sprint(req["notes"])
		if err := n.createRelease(group, repo, tag, notes); err != nil {
			return StatusResponse(ResRemoteFail, "Could not create release")
		}
		return []byte{ResOK}
	}
	return StatusResponse(ResInvalidReq, "Invalid request")
}

func (n *Node) handleWork(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	req, err := DecodeRequest(data)
	if err != nil {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	repoPath, ok := RepoFromRequest(req)
	if !ok {
		return StatusResponse(ResInvalidReq, "No repository specified")
	}
	group, repo, ok := ParseRepoPath(repoPath)
	if !ok {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	op := fmt.Sprint(req["operation"])
	switch op {
	case "list":
		if !n.remoteAllowed(remote, group, repo, permRead) {
			return StatusResponse(ResNotFound, "Not found")
		}
		body := n.listWorkDocs(group)
		return append([]byte{ResOK}, body...)
	case "view":
		if !n.remoteAllowed(remote, group, repo, permRead) {
			return StatusResponse(ResNotFound, "Not found")
		}
		docID := fmt.Sprint(req["doc_id"])
		body, err := n.readWorkDoc(group, docID)
		if err != nil {
			return StatusResponse(ResNotFound, "Not found")
		}
		return append([]byte{ResOK}, body...)
	case "create", "propose":
		perm := permWrite
		if op == "propose" {
			perm = permPropose
		}
		if !n.remoteAllowed(remote, group, repo, perm) {
			return StatusResponse(ResDisallowed, "Not allowed")
		}
		title := fmt.Sprint(req["title"])
		content := fmt.Sprint(req["content"])
		id, err := n.createWorkDoc(group, title, content, remote)
		if err != nil {
			return StatusResponse(ResRemoteFail, "Could not create work document")
		}
		return append([]byte{ResOK}, []byte(id)...)
	}
	return StatusResponse(ResInvalidReq, "Invalid request")
}

func (n *Node) releasesDir(group, repo string) string {
	ga, ok := n.access.Groups[group]
	if !ok {
		return ""
	}
	return filepath.Join(filepath.Dir(ga.Path), repo+".releases")
}

func (n *Node) listReleases(group, repo string) ([]byte, error) {
	dir := n.releasesDir(group, repo)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte("Tag Status Created\n"), nil
		}
		return nil, err
	}
	var b strings.Builder
	b.WriteString("Tag Status Created\n")
	b.WriteString(strings.Repeat("-", 66))
	b.WriteByte('\n')
	for _, ent := range entries {
		if ent.IsDir() {
			b.WriteString(ent.Name())
			b.WriteString(" published\n")
		}
	}
	return []byte(b.String()), nil
}

func (n *Node) fetchReleaseArtifact(group, repo, tag, artifact string) ([]byte, error) {
	path := filepath.Join(n.releasesDir(group, repo), tag, "artifacts", artifact)
	return os.ReadFile(path) // #nosec G304 -- release artifact path
}

func (n *Node) createRelease(group, repo, tag, notes string) error {
	dir := filepath.Join(n.releasesDir(group, repo), tag)
	if err := os.MkdirAll(filepath.Join(dir, "artifacts"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "RELEASE.md"), []byte(notes), 0o644)
}

func (n *Node) workDir(group string) string {
	ga, ok := n.access.Groups[group]
	if !ok {
		return ""
	}
	return filepath.Join(filepath.Dir(ga.Path), group+".work")
}

func (n *Node) listWorkDocs(group string) []byte {
	dir := n.workDir(group)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []byte("ID Title Author\n")
	}
	var b strings.Builder
	b.WriteString("ID Title Author\n")
	for _, ent := range entries {
		if ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		b.WriteString(ent.Name())
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func (n *Node) readWorkDoc(group, docID string) ([]byte, error) {
	return os.ReadFile(filepath.Join(n.workDir(group), docID+".txt")) // #nosec G304 -- work doc id
}

func (n *Node) createWorkDoc(group, title, content string, remote *identity.Identity) (string, error) {
	dir := n.workDir(group)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	id := fmt.Sprintf("%d", time.Now().Unix())
	body := title + "\n" + content + "\nauthor:" + hex.EncodeToString(remote.Hash())
	path := filepath.Join(dir, id+".txt")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return id, nil
}
