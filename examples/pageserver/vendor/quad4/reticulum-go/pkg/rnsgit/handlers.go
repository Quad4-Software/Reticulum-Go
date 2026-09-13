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

func (n *Node) handleList(path string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	if remote == nil {
		return StatusResponse(ResDisallowed, "Not identified")
	}
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
	forPush := false
	if v, ok := req["for_push"]; ok {
		if b, ok := v.(bool); ok {
			forPush = b
		}
	}
	perm := permRead
	if forPush {
		perm = permWrite
	}
	if !n.remoteAllowed(remote, group, repo, perm) {
		if n.remoteAllowed(remote, group, repo, permRead) {
			return StatusResponse(ResNotFound, "Not allowed")
		}
		return StatusResponse(ResNotFound, "Not found")
	}
	p, ok := n.repoPath(group, repo)
	if !ok {
		return StatusResponse(ResNotFound, "Not found")
	}
	body, err := n.git.ListRefs(p)
	if err != nil {
		return StatusResponse(ResRemoteFail, "Could not list refs")
	}
	out := append([]byte{ResOK}, []byte(body)...)
	return out
}

func (n *Node) handleFetch(path string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	if remote == nil {
		return StatusResponse(ResDisallowed, "Not identified")
	}
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
	if !n.remoteAllowed(remote, group, repo, permRead) {
		return StatusResponse(ResNotFound, "Not found")
	}
	p, ok := n.repoPath(group, repo)
	if !ok {
		return StatusResponse(ResNotFound, "Not found")
	}
	refsAny, ok := req["refs"]
	if !ok {
		return StatusResponse(ResInvalidReq, "No refs specified")
	}
	refsList, ok := refsAny.([]any)
	if !ok {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	refs := make([]map[string]string, 0, len(refsList))
	names := make([]string, 0, len(refsList))
	for _, item := range refsList {
		m, ok := item.(map[any]any)
		if !ok {
			m2, ok := item.(map[string]any)
			if !ok {
				return StatusResponse(ResInvalidReq, "Invalid request")
			}
			m = map[any]any{}
			for k, v := range m2 {
				m[k] = v
			}
		}
		ref := fmt.Sprint(m["ref"])
		if SanRef(ref) == "" {
			return StatusResponse(ResInvalidReq, "Invalid request")
		}
		entry := map[string]string{"ref": ref}
		if hv, ok := m["have"]; ok {
			entry["have"] = fmt.Sprint(hv)
		}
		refs = append(refs, entry)
		names = append(names, ref)
	}
	if SanRefs(names) == nil {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	var have []string
	if hv, ok := req["have"]; ok {
		if arr, ok := hv.([]any); ok {
			for _, x := range arr {
				if s := SanSHA(fmt.Sprint(x)); s != "" {
					have = append(have, s)
				}
			}
		}
	}
	tmp, err := os.MkdirTemp("", "rnsgit-fetch-")
	if err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	defer os.RemoveAll(tmp)
	bundlePath := filepath.Join(tmp, "fetch.bundle")
	if err := n.git.CreateBundle(p, bundlePath, refs, have); err != nil {
		if IsEmptyBundle(err) {
			return []byte{ResOK}
		}
		return StatusResponse(ResRemoteFail, "Could not fetch refs")
	}
	bundle, err := os.ReadFile(bundlePath) // #nosec G304 -- temp bundle
	if err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	return link.FileResponse{Data: bundle, MetadataPacked: OKMetadataPacked(), AutoCompress: true}
}

func (n *Node) handlePush(path string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	if remote == nil {
		return StatusResponse(ResDisallowed, "Not identified")
	}
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
	if !n.remoteAllowed(remote, group, repo, permWrite) {
		if n.remoteAllowed(remote, group, repo, permRead) {
			return StatusResponse(ResDisallowed, "Not allowed")
		}
		return StatusResponse(ResNotFound, "Not found")
	}
	p, ok := n.repoPath(group, repo)
	if !ok {
		return StatusResponse(ResNotFound, "Not found")
	}
	if bundle, ok := req["bundle"]; ok {
		localRef := SanRef(fmt.Sprint(req["local_ref"]))
		remoteRef := SanRef(fmt.Sprint(req["remote_ref"]))
		force, _ := req["force"].(bool)
		if localRef == "" || remoteRef == "" {
			return StatusResponse(ResInvalidReq, "Missing ref specification")
		}
		var bundleData []byte
		switch b := bundle.(type) {
		case []byte:
			bundleData = b
		case string:
			bundleData = []byte(b)
		default:
			return StatusResponse(ResInvalidReq, "Invalid bundle")
		}
		tmp, err := os.MkdirTemp("", "rnsgit-push-")
		if err != nil {
			return StatusResponse(ResRemoteFail, "Remote error")
		}
		defer os.RemoveAll(tmp)
		bundlePath := filepath.Join(tmp, "push.bundle")
		if err := os.WriteFile(bundlePath, bundleData, 0o600); err != nil {
			return StatusResponse(ResRemoteFail, "Remote error")
		}
		if err := n.git.VerifyBundle(p, bundlePath); err != nil {
			return StatusResponse(ResRemoteFail, "Could not verify bundle")
		}
		if err := n.git.FetchBundle(p, bundlePath, localRef, remoteRef, force); err != nil {
			return StatusResponse(ResRemoteFail, "Could not verify bundle")
		}
		return []byte{ResOK}
	}
	if ops, ok := req["operations"]; ok {
		list, ok := ops.([]any)
		if !ok {
			return StatusResponse(ResInvalidReq, "Invalid data for operations")
		}
		for _, item := range list {
			m, ok := item.(map[any]any)
			if !ok {
				m2, ok := item.(map[string]any)
				if !ok {
					return StatusResponse(ResInvalidReq, "Invalid request")
				}
				m = map[any]any{}
				for k, v := range m2 {
					m[k] = v
				}
			}
			if fmt.Sprint(m["action"]) != "update_ref" {
				return StatusResponse(ResInvalidReq, "Unknown operation")
			}
			ref := SanRef(fmt.Sprint(m["ref"]))
			sha := SanSHA(fmt.Sprint(m["sha"]))
			force, _ := m["force"].(bool)
			if ref == "" || sha == "" || !strings.HasPrefix(ref, "refs/") {
				return StatusResponse(ResInvalidReq, "Invalid request")
			}
			if !n.git.ObjectExists(p, sha) {
				return StatusResponse(ResRemoteFail, "Object does not exist in repository")
			}
			if err := n.git.UpdateRef(p, ref, sha, force); err != nil {
				if strings.Contains(err.Error(), "different sha") {
					return StatusResponse(ResDisallowed, "Ref exists at different SHA (force required)")
				}
				return StatusResponse(ResRemoteFail, "Could not update refs")
			}
		}
		return []byte{ResOK}
	}
	return StatusResponse(ResInvalidReq, "Invalid request data")
}

func (n *Node) handleDelete(path string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	if remote == nil {
		return StatusResponse(ResDisallowed, "Not identified")
	}
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
	if !n.remoteAllowed(remote, group, repo, permWrite) {
		if n.remoteAllowed(remote, group, repo, permRead) {
			return StatusResponse(ResDisallowed, "Not allowed")
		}
		return StatusResponse(ResNotFound, "Not found")
	}
	p, ok := n.repoPath(group, repo)
	if !ok {
		return StatusResponse(ResNotFound, "Not found")
	}
	ref := SanRef(fmt.Sprint(req["ref"]))
	if ref == "" || !strings.HasPrefix(ref, "refs/") {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	if err := n.git.DeleteRef(p, ref); err != nil {
		return StatusResponse(ResRemoteFail, "Could not delete ref")
	}
	return []byte{ResOK}
}

func (n *Node) handleCreate(path string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	if remote == nil {
		return StatusResponse(ResDisallowed, "Not identified")
	}
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
	tab := n.accessTable()
	ga, ok := tab.Groups[group]
	if !ok {
		return StatusResponse(ResNotFound, "Not found")
	}
	if _, err := os.Stat(ga.Path); err != nil {
		return StatusResponse(ResNotFound, "Not found")
	}
	remoteHash := remote.Hash()
	if !tab.Resolve(group, "", remoteHash, permCreate) {
		if tab.Resolve(group, "", remoteHash, permRead) {
			return StatusResponse(ResDisallowed, "Not allowed")
		}
		return StatusResponse(ResNotFound, "Not found")
	}
	repoDir := filepath.Join(ga.Path, repo)
	_, statErr := os.Stat(repoDir)
	_, registered := ga.Repositories[repo]
	if statErr == nil || registered {
		if tab.Resolve(group, repo, remoteHash, permRead) {
			return StatusResponse(ResDisallowed, "Repository already exists")
		}
		return StatusResponse(ResNotFound, "Not found")
	}
	if err := n.git.InitBare(repoDir); err != nil {
		_ = os.RemoveAll(repoDir)
		return StatusResponse(ResRemoteFail, "Could not initialize repository")
	}
	if err := GrantCreatorAdmin(repoDir+".allowed", hex.EncodeToString(remoteHash)); err != nil {
		_ = os.RemoveAll(repoDir)
		return StatusResponse(ResRemoteFail, "Could not initialize repository")
	}
	_ = n.reloadAccess()
	return []byte{ResOK}
}

func (n *Node) handleFork(path string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	return n.remoteClone(path, data, remote, "fork")
}

func (n *Node) handleMirror(path string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	return n.remoteClone(path, data, remote, "mirror")
}

// cloneProtos lists the permitted upstream URL schemes, matching Python
// CLONE_PROTOS.
var cloneProtos = map[string]bool{"rns": true, "http": true, "https": true, "ssh": true}

func (n *Node) remoteClone(_ string, data []byte, remote *identity.Identity, repoType string) any {
	if repoType != "mirror" && repoType != "fork" {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	if remote == nil {
		return StatusResponse(ResDisallowed, "Not identified")
	}
	req, err := DecodeRequest(data)
	if err != nil {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	repoPath, ok := RepoFromRequest(req)
	if !ok {
		return StatusResponse(ResInvalidReq, "No repository specified")
	}
	source, isStr := req["source"].(string)
	if !isStr || source == "" {
		if b, isBytes := req["source"].([]byte); !isBytes || len(b) == 0 {
			return StatusResponse(ResInvalidReq, "No source specified")
		}
		return StatusResponse(ResInvalidReq, "Invalid source URL")
	}
	if scheme := strings.ToLower(strings.SplitN(source, "://", 2)[0]); !cloneProtos[scheme] {
		return StatusResponse(ResDisallowed, "Prohibited source URL")
	}
	group, repo, ok := ParseRepoPath(repoPath)
	if !ok {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	tab := n.accessTable()
	ga, ok := tab.Groups[group]
	if !ok {
		return StatusResponse(ResNotFound, "Not found")
	}
	if _, err := os.Stat(ga.Path); err != nil {
		return StatusResponse(ResNotFound, "Not found")
	}
	remoteHash := remote.Hash()
	if !tab.Resolve(group, "", remoteHash, permCreate) {
		if !tab.Resolve(group, "", remoteHash, permRead) {
			return StatusResponse(ResNotFound, "Not found")
		}
		return StatusResponse(ResDisallowed, "Not allowed")
	}
	repoDir := filepath.Join(ga.Path, repo)
	_, registered := ga.Repositories[repo]
	if _, err := os.Stat(repoDir); err == nil || registered {
		if tab.Resolve(group, repo, remoteHash, permRead) {
			return StatusResponse(ResDisallowed, "Repository already exists")
		}
		return StatusResponse(ResNotFound, "Not found")
	}
	tmp, err := os.MkdirTemp(ga.Path, ".rngit-clone-")
	if err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	defer os.RemoveAll(tmp) // #nosec G104 -- clone scratch dir
	tmpRepo := filepath.Join(tmp, repo)
	if err := n.git.InitBare(tmpRepo); err != nil {
		return StatusResponse(ResRemoteFail, "Failed to initialize repository")
	}
	if err := n.git.FetchAll(tmpRepo, source); err != nil {
		return StatusResponse(ResRemoteFail, "Failed to fetch from source")
	}
	n.git.UpdateHeadToSourceDefault(tmpRepo, source)
	if err := n.git.SetConfig(tmpRepo, "repository.rngit.type", repoType); err != nil {
		return StatusResponse(ResRemoteFail, "Could not configure repository")
	}
	if err := n.git.SetConfig(tmpRepo, "repository.rngit.upstream.source", source); err != nil {
		return StatusResponse(ResRemoteFail, "Could not configure repository")
	}
	if err := n.git.SetConfig(tmpRepo, "repository.rngit.upstream.sync", fmt.Sprintf("%d", time.Now().Unix())); err != nil {
		return StatusResponse(ResRemoteFail, "Could not configure repository")
	}
	if err := GrantCreatorAdmin(repoDir+".allowed", hex.EncodeToString(remoteHash)); err != nil {
		return StatusResponse(ResRemoteFail, "Could not initialize repository")
	}
	if err := os.Rename(tmpRepo, repoDir); err != nil {
		return StatusResponse(ResRemoteFail, "Could not write repository")
	}
	_ = n.reloadAccess()
	return []byte{ResOK}
}

func (n *Node) handleSync(path string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	if remote == nil {
		return StatusResponse(ResDisallowed, "Not identified")
	}
	req, err := DecodeRequest(data)
	if err != nil {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	repoPath, ok := RepoFromRequest(req)
	if !ok {
		return StatusResponse(ResInvalidReq, "No repository specified")
	}
	group, repo, _ := ParseRepoPath(repoPath)
	tab := n.accessTable()
	if !tab.Resolve(group, repo, remote.Hash(), permRead) {
		return StatusResponse(ResNotFound, "Not found")
	}
	if !tab.Resolve(group, repo, remote.Hash(), permWrite) {
		return StatusResponse(ResDisallowed, "Not allowed")
	}
	p, ok := n.repoPath(group, repo)
	if !ok {
		return StatusResponse(ResNotFound, "Not found")
	}
	repoType, _ := n.git.ConfigValue(p, "repository.rngit.type")
	if repoType != "mirror" && repoType != "fork" {
		return StatusResponse(ResInvalidReq, "Repository is neither fork nor mirror")
	}
	source, err := n.git.ConfigValue(p, "repository.rngit.upstream.source")
	if err != nil || source == "" {
		return StatusResponse(ResRemoteFail, "Sync failed")
	}
	if err := n.git.FetchAll(p, source); err != nil {
		if repoType == "mirror" {
			return StatusResponse(ResRemoteFail, "Mirror sync failed")
		}
		return StatusResponse(ResRemoteFail, "Fork sync failed")
	}
	if repoType == "mirror" {
		n.git.UpdateHeadToSourceDefault(p, source)
	}
	_ = n.git.SetConfig(p, "repository.rngit.upstream.sync", fmt.Sprintf("%d", time.Now().Unix()))
	return []byte{ResOK}
}
