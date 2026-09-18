// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"fmt"
	"strings"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

// handlePerms serves /mgmt/perms requests, matching Python handle_perms.
func (n *Node) handlePerms(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	if remote == nil {
		return StatusResponse(ResDisallowed, "Not identified")
	}
	req, err := DecodeRequest(data)
	if err != nil {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	op := reqString(req, "operation")
	if op == "" {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	access := n.accessTable()
	switch op {
	case "gperms":
		gv, ok := reqIntKey(req, IdxGroup)
		if !ok {
			return StatusResponse(ResInvalidReq, "No group specified")
		}
		group := reqStringVal(gv)
		if group == "" || len(group) > 256 || strings.Contains(group, "/") {
			return StatusResponse(ResNotFound, "Not found")
		}
		if !access.Resolve(group, "", remote.Hash(), permRead) {
			return StatusResponse(ResNotFound, "Not found")
		}
		if !access.Resolve(group, "", remote.Hash(), permAdmin) {
			return StatusResponse(ResDisallowed, "Not allowed")
		}
		ga, ok := access.Groups[group]
		if !ok {
			return StatusResponse(ResNotFound, "Not found")
		}
		return n.setPermissions(ga.Path+".allowed", req)
	case "rperms":
		repoPath, ok := RepoFromRequest(req)
		if !ok {
			return StatusResponse(ResInvalidReq, "No repository specified")
		}
		group, repo, _ := ParseRepoPath(repoPath)
		if !access.Resolve(group, repo, remote.Hash(), permRead) {
			return StatusResponse(ResNotFound, "Not found")
		}
		if !access.Resolve(group, repo, remote.Hash(), permAdmin) {
			return StatusResponse(ResDisallowed, "Not allowed")
		}
		ga, ok := access.Groups[group]
		if !ok {
			return StatusResponse(ResNotFound, "Not found")
		}
		ra, ok := ga.Repositories[repo]
		if !ok {
			return StatusResponse(ResNotFound, "Not found")
		}
		return n.setPermissions(ra.Path+".allowed", req)
	}
	return StatusResponse(ResInvalidReq, "Invalid request")
}

// setPermissions implements the shared get/set logic of Python
// _group_permissions and _repository_permissions.
func (n *Node) setPermissions(allowedPath string, req map[any]any) any {
	step := reqString(req, "step")
	if step == "" {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	switch step {
	case "get":
		content, _ := ReadAllowedFile(allowedPath)
		return PermsGetResponse(content)
	case "set":
		content := reqString(req, "content")
		if invalid, line, ok := ValidateAllowedContent(content, n.accessTable().Aliases); !ok {
			return StatusResponse(ResInvalidReq, fmt.Sprintf("Invalid permission %q on line %d", invalid, line))
		}
		if IsExecutableResolver(allowedPath) {
			return StatusResponse(ResDisallowed, "Executable permission resolvers can only be modified node-side")
		}
		if err := WriteAllowedFile(allowedPath, content); err != nil {
			return StatusResponse(ResRemoteFail, "Error setting permissions")
		}
		_ = n.reloadAccess()
		return []byte{ResOK}
	}
	return StatusResponse(ResInvalidReq, "Invalid step")
}

// handleRelease serves /mgmt/release requests, matching Python
// handle_release and the _release_* handlers.
func (n *Node) handleRelease(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	if remote == nil {
		return StatusResponse(ResDisallowed, "Not identified")
	}
	req, err := DecodeRequest(data)
	if err != nil {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	if _, ok := RepoFromRequest(req); !ok {
		return StatusResponse(ResInvalidReq, "No repository specified")
	}
	op := reqString(req, "operation")
	if op == "" {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	repoPath, _ := RepoFromRequest(req)
	group, repo, _ := ParseRepoPath(repoPath)
	tab := n.accessTable()
	remoteHash := remote.Hash()
	readAccess := tab.Resolve(group, repo, remoteHash, permRead)
	releaseAccess := tab.Resolve(group, repo, remoteHash, permRelease)
	if !readAccess {
		return StatusResponse(ResNotFound, "Not found")
	}
	var access bool
	switch op {
	case "create", "delete", "latest":
		access = releaseAccess && readAccess
	case "list", "view", "fetch":
		access = readAccess
	}
	if !access {
		return StatusResponse(ResDisallowed, "Not allowed")
	}
	repositoryPath, ok := n.repoPath(group, repo)
	if !ok {
		return StatusResponse(ResNotFound, "Not found")
	}
	releasesPath := repositoryPath + ".releases"
	switch op {
	case "list":
		return n.releaseList(releasesPath)
	case "view":
		return n.releaseView(releasesPath, req)
	case "fetch":
		return n.releaseFetch(releasesPath, req)
	case "create":
		return n.releaseCreate(releasesPath, repositoryPath, req, remote)
	case "delete":
		return n.releaseDelete(releasesPath, req)
	case "latest":
		return n.releaseLatest(releasesPath, req)
	}
	return StatusResponse(ResInvalidReq, "Invalid request")
}
