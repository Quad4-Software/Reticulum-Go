// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

// workDocLimit matches Python WORK_DOC_LIMIT.
const workDocLimit = 256 * 1024

// workScopes are the rngit work document scopes.
var workScopes = []string{"active", "completed", "proposed"}

func workScopesValid(scope string) bool {
	switch scope {
	case "active", "completed", "proposed", "all":
		return true
	}
	return false
}

// workPath returns the repository work document root, matching Python
// work_path of repository_path + ".work".
func (n *Node) workPath(group, repo string) (string, bool) {
	repoPath, ok := n.repoPath(group, repo)
	if !ok {
		return "", false
	}
	return repoPath + ".work", true
}

// workDocDir locates a document directory across scopes.
func workDocDir(workPath string, docID int, scopes []string) (scope, dir string, ok bool) {
	for _, s := range scopes {
		d := filepath.Join(workPath, s, strconv.Itoa(docID))
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return s, d, true
		}
	}
	return "", "", false
}

// workNextID returns the next document id across all scopes, matching Python
// _work_get_next_id.
func workNextID(workPath string) int {
	next := 1
	for _, s := range workScopes {
		if n := workDirNextID(filepath.Join(workPath, s)); n > next {
			next = n
		}
	}
	return next
}

// workDirNextID returns the next numeric entry id inside dir, matching Python
// _work_get_next_comment_id and scope_next_id.
func workDirNextID(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 1
	}
	max := 0
	for _, ent := range entries {
		if n, err := strconv.Atoi(ent.Name()); err == nil && n > max {
			max = n
		}
	}
	return max + 1
}

// saveWorkDoc persists a document as msgpack via tmp+rename, matching Python
// _work_save_document.
func saveWorkDoc(path string, doc map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- work doc dir
		return err
	}
	packed, err := msgpack.Marshal(doc)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, packed, 0o644); err != nil { // #nosec G306 -- work doc
		return err
	}
	return os.Rename(tmp, path)
}

// loadWorkDoc reads a msgpack work document, matching Python
// _work_load_document.
func loadWorkDoc(path string) (map[string]any, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- work doc
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := msgpack.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func docMeta(doc map[string]any) map[string]any {
	if m, ok := doc["meta"].(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func metaString(meta map[string]any, key, def string) string {
	if v, ok := meta[key]; ok {
		switch s := v.(type) {
		case string:
			return s
		case []byte:
			return string(s)
		}
	}
	return def
}

func metaBytes(meta map[string]any, key string) []byte {
	if v, ok := meta[key]; ok {
		if b, ok := v.([]byte); ok {
			return b
		}
	}
	return nil
}

func metaFloat(meta map[string]any, key string) float64 {
	switch v := meta[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int:
		return float64(v)
	case uint64:
		return float64(v)
	}
	return 0
}

func metaAuthorHex(meta map[string]any) string {
	author := metaBytes(meta, "author")
	if len(author) == 0 {
		return ""
	}
	return hex.EncodeToString(author)
}

// workDocID parses a request doc_id value, matching Python int(doc_id).
func workDocID(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int8:
		return int(x), true
	case int16:
		return int(x), true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case uint8:
		return int(x), true
	case uint16:
		return int(x), true
	case uint32:
		return int(x), true
	case uint64:
		if x > math.MaxInt {
			return 0, false
		}
		return int(x), true
	case float64:
		return int(x), true
	case float32:
		return int(x), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		if err != nil {
			return 0, false
		}
		return n, true
	case []byte:
		n, err := strconv.Atoi(strings.TrimSpace(string(x)))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func workOK(payload any) []byte {
	packed, err := msgpack.Marshal(payload)
	if err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	return append([]byte{ResOK}, packed...)
}

// handleWork serves /mgmt/work requests, matching Python handle_work.
func (n *Node) handleWork(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
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
	operation := fmt.Sprint(req["operation"])
	if operation == "" || operation == "<nil>" {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	group, repo, _ := ParseRepoPath(repoPath)

	remoteHash := remote.Hash()
	readAccess := n.accessTable().Resolve(group, repo, remoteHash, permRead)
	writeAccess := n.accessTable().Resolve(group, repo, remoteHash, permWrite)
	interactAccess := n.accessTable().Resolve(group, repo, remoteHash, permInteract)
	proposeAccess := n.accessTable().Resolve(group, repo, remoteHash, permPropose)
	adminAccess := n.accessTable().Resolve(group, repo, remoteHash, permAdmin)

	if !readAccess {
		return StatusResponse(ResNotFound, "Not found")
	}

	docID, hasDocID := workDocID(req["doc_id"])
	docScoped := map[string]bool{
		"read": true, "view": true, "comment": true,
		"edit": true, "delete": true, "perms": true,
	}
	if docScoped[operation] && hasDocID {
		readAccess = n.accessTable().ResolveDoc(group, repo, docID, remoteHash, permRead) || adminAccess
		if !readAccess {
			return StatusResponse(ResNotFound, "Document not found")
		}
	}
	if operation == "comment" && hasDocID {
		interactAccess = interactAccess || n.accessTable().ResolveDoc(group, repo, docID, remoteHash, permInteract)
	}
	if operation == "edit" && hasDocID {
		interactAccess = interactAccess || n.accessTable().ResolveDoc(group, repo, docID, remoteHash, permInteract)
		writeAccess = writeAccess || n.accessTable().ResolveDoc(group, repo, docID, remoteHash, permWrite)
	}

	commentAccess := interactAccess && (readAccess || writeAccess)
	manageAccess := interactAccess && writeAccess

	var access bool
	switch operation {
	case "list", "view":
		access = readAccess
	case "comment":
		access = commentAccess
	case "propose":
		access = proposeAccess
	case "create", "edit", "delete", "complete", "activate":
		access = manageAccess
	case "perms":
		access = adminAccess
	default:
		access = false
	}
	if !access {
		return StatusResponse(ResDisallowed, "Not allowed")
	}

	workPath, ok := n.workPath(group, repo)
	if !ok {
		return StatusResponse(ResNotFound, "Not found")
	}

	n.workMu.Lock()
	defer n.workMu.Unlock()

	switch operation {
	case "list":
		return n.workList(workPath, group, repo, req, remote)
	case "view":
		return workView(workPath, req)
	case "comment":
		return n.workComment(workPath, req, remote)
	case "create":
		return n.workCreate(workPath, "active", req, remote)
	case "propose":
		return n.workPropose(workPath, req, remote)
	case "edit":
		return n.workEdit(workPath, req, remote)
	case "delete":
		return n.workDelete(workPath, group, repo, req, remote)
	case "complete":
		return n.workComplete(workPath, group, repo, req, remote)
	case "activate":
		return n.workActivate(workPath, group, repo, req, remote)
	case "perms":
		return n.workPerms(workPath, group, repo, req, remote)
	}
	return StatusResponse(ResInvalidReq, "Invalid request")
}

// workList implements Python _work_list.
func (n *Node) workList(workPath, group, repo string, req map[any]any, remote *identity.Identity) any {
	scope := fmt.Sprint(req["scope"])
	if scope == "<nil>" {
		scope = "active"
	}
	result := map[string]any{
		"active":    []any{},
		"completed": []any{},
		"proposed":  []any{},
	}
	for _, key := range workScopes {
		if scope != key && scope != "all" {
			continue
		}
		folder := filepath.Join(workPath, key)
		entries, err := os.ReadDir(folder)
		if err != nil {
			continue
		}
		list := []any{}
		for _, ent := range entries {
			docDir := filepath.Join(folder, ent.Name())
			if !ent.IsDir() {
				continue
			}
			docID, err := strconv.Atoi(ent.Name())
			if err != nil {
				continue
			}
			if !n.accessTable().ResolveDoc(group, repo, docID, remote.Hash(), permRead) {
				continue
			}
			rootPath := filepath.Join(docDir, "root")
			if st, err := os.Stat(rootPath); err != nil || !st.Mode().IsRegular() {
				continue
			}
			doc, err := loadWorkDoc(rootPath)
			if err != nil {
				continue
			}
			meta := docMeta(doc)
			comments := 0
			for _, cent := range entriesOfDir(docDir) {
				if cent.IsDir() {
					continue
				}
				if _, err := strconv.Atoi(cent.Name()); err == nil {
					comments++
				}
			}
			list = append(list, map[string]any{
				"id":       docID,
				"title":    metaString(meta, "title", "Untitled"),
				"created":  metaFloat(meta, "created"),
				"edited":   metaFloat(meta, "edited"),
				"author":   metaAuthorHex(meta),
				"format":   metaString(meta, "format", "markdown"),
				"comments": comments,
			})
		}
		sort.Slice(list, func(i, j int) bool {
			a, _ := list[i].(map[string]any)["created"].(float64)
			b, _ := list[j].(map[string]any)["created"].(float64)
			return a > b
		})
		result[key] = list
	}
	return workOK(result)
}

func entriesOfDir(dir string) []os.DirEntry {
	entries, _ := os.ReadDir(dir)
	return entries
}

// workView implements Python _work_view.
func workView(workPath string, req map[any]any) any {
	scope := fmt.Sprint(req["scope"])
	if scope == "<nil>" {
		scope = "all"
	}
	if !workScopesValid(scope) {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	v := req["doc_id"]
	if v == nil {
		return StatusResponse(ResInvalidReq, "No document ID specified")
	}
	docID, ok := workDocID(v)
	if !ok {
		return StatusResponse(ResInvalidReq, "Invalid document ID")
	}
	foundScope, docDir, found := workDocDir(workPath, docID, workScopes)
	if !found {
		return StatusResponse(ResNotFound, "Not found")
	}
	rootPath := filepath.Join(docDir, "root")
	if st, err := os.Stat(rootPath); err != nil || !st.Mode().IsRegular() {
		return StatusResponse(ResNotFound, "Document not found")
	}
	doc, err := loadWorkDoc(rootPath)
	if err != nil {
		return StatusResponse(ResRemoteFail, "Error loading document")
	}
	var comments []any
	for _, ent := range entriesOfDir(docDir) {
		if ent.IsDir() {
			continue
		}
		commentID, err := strconv.Atoi(ent.Name())
		if err != nil {
			continue
		}
		comment, err := loadWorkDoc(filepath.Join(docDir, ent.Name()))
		if err != nil {
			continue
		}
		cmeta := docMeta(comment)
		comments = append(comments, map[string]any{
			"id":      commentID,
			"content": metaString(map[string]any{"c": comment["content"]}, "c", ""),
			"created": metaFloat(cmeta, "created"),
			"edited":  metaFloat(cmeta, "edited"),
			"author":  metaAuthorHex(cmeta),
			"format":  metaString(cmeta, "format", "markdown"),
		})
	}
	sort.Slice(comments, func(i, j int) bool {
		a, _ := comments[i].(map[string]any)["id"].(int)
		b, _ := comments[j].(map[string]any)["id"].(int)
		return a < b
	})
	meta := docMeta(doc)
	result := map[string]any{
		"id":       docID,
		"scope":    foundScope,
		"content":  doc["content"],
		"comments": comments,
		"meta": map[string]any{
			"title":     metaString(meta, "title", "Untitled"),
			"created":   metaFloat(meta, "created"),
			"edited":    metaFloat(meta, "edited"),
			"author":    metaAuthorHex(meta),
			"identity":  metaBytes(meta, "identity"),
			"signature": metaBytes(meta, "signature"),
			"format":    metaString(meta, "format", "markdown"),
		},
	}
	return workOK(result)
}

// workValidateSigned validates create/propose signature fields, matching
// Python signature checks.
func workValidateSigned(remote *identity.Identity, title, content, format string, signature []byte) []byte {
	if len(signature) == 0 {
		return StatusResponse(ResInvalidReq, "No signature provided")
	}
	if len(signature) != 64 {
		return StatusResponse(ResInvalidReq, "Invalid signature length")
	}
	if !remote.Verify([]byte(content), signature) {
		return StatusResponse(ResInvalidReq, "Invalid signature")
	}
	if len(title)+len(content)+len(format) > workDocLimit {
		return StatusResponse(ResInvalidReq, "Content limit exceeded")
	}
	if title == "" {
		return StatusResponse(ResInvalidReq, "Title is required")
	}
	if content == "" {
		return StatusResponse(ResInvalidReq, "Content is required")
	}
	return nil
}

func workFormat(format string) string {
	if format == "markdown" || format == "micron" {
		return format
	}
	return "markdown"
}

// workCreate implements Python _work_create and _work_propose for scope.
func (n *Node) workCreate(workPath, scope string, req map[any]any, remote *identity.Identity) any {
	title := strings.TrimSpace(fmt.Sprint(req["title"]))
	content := strings.TrimSpace(reqString(req, "content"))
	format := reqString(req, "format")
	if format == "" || format == "<nil>" {
		format = "markdown"
	}
	signature := reqBytes(req, "signature")
	if resp := workValidateSigned(remote, title, content, format, signature); resp != nil {
		return resp
	}
	docID := workNextID(workPath)
	docDir := filepath.Join(workPath, scope, strconv.Itoa(docID))
	now := float64(time.Now().UnixNano()) / 1e9
	doc := map[string]any{
		"content": content,
		"meta": map[string]any{
			"format":    workFormat(format),
			"title":     title,
			"created":   now,
			"edited":    now,
			"author":    remote.Hash(),
			"signature": signature,
			"identity":  remote.GetPublicKey(),
		},
	}
	if err := saveWorkDoc(filepath.Join(docDir, "root"), doc); err != nil {
		return StatusResponse(ResRemoteFail, "Error saving document")
	}
	return workOK(map[string]any{"id": docID, "scope": scope})
}

// workPropose implements Python _work_propose including the owner .allowed
// file grant.
func (n *Node) workPropose(workPath string, req map[any]any, remote *identity.Identity) any {
	title := strings.TrimSpace(fmt.Sprint(req["title"]))
	content := strings.TrimSpace(reqString(req, "content"))
	format := reqString(req, "format")
	if format == "" || format == "<nil>" {
		format = "markdown"
	}
	signature := reqBytes(req, "signature")
	if resp := workValidateSigned(remote, title, content, format, signature); resp != nil {
		return resp
	}
	docID := workNextID(workPath)
	docDir := filepath.Join(workPath, "proposed", strconv.Itoa(docID))
	now := float64(time.Now().UnixNano()) / 1e9
	doc := map[string]any{
		"content": content,
		"meta": map[string]any{
			"format":    workFormat(format),
			"title":     title,
			"created":   now,
			"edited":    now,
			"author":    remote.Hash(),
			"signature": signature,
			"identity":  remote.GetPublicKey(),
		},
	}
	if err := saveWorkDoc(filepath.Join(docDir, "root"), doc); err != nil {
		return StatusResponse(ResRemoteFail, "Error saving document")
	}
	ownerHex := hex.EncodeToString(remote.Hash())
	ownerPerms := "i:" + ownerHex + "\nw:" + ownerHex + "\n"
	allowedPath := filepath.Join(workPath, strconv.Itoa(docID)+".allowed")
	if err := WriteAllowedFile(allowedPath, ownerPerms); err != nil {
		return StatusResponse(ResRemoteFail, "Error setting document ownership")
	}
	return workOK(map[string]any{"id": docID, "scope": "proposed"})
}

// workEdit implements Python _work_edit.
func (n *Node) workEdit(workPath string, req map[any]any, remote *identity.Identity) any {
	scope := fmt.Sprint(req["scope"])
	if scope == "<nil>" {
		scope = "active"
	}
	if !workScopesValid(scope) {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	content := reqString(req, "content")
	title := reqString(req, "title")
	signature := reqBytes(req, "signature")
	size := len(title) + len(content)
	if len(signature) == 0 {
		return StatusResponse(ResInvalidReq, "No signature provided")
	}
	if len(signature) != 64 {
		return StatusResponse(ResInvalidReq, "Invalid signature length")
	}
	if !remote.Verify([]byte(content), signature) {
		return StatusResponse(ResInvalidReq, "Invalid signature")
	}
	if size > workDocLimit {
		return StatusResponse(ResInvalidReq, "Content limit exceeded")
	}
	if content == "" && title == "" {
		return StatusResponse(ResInvalidReq, "No changes specified")
	}
	v := req["doc_id"]
	if v == nil {
		return StatusResponse(ResInvalidReq, "No document ID specified")
	}
	docID, ok := workDocID(v)
	if !ok {
		return StatusResponse(ResInvalidReq, "Invalid document ID")
	}
	_, docDir, found := workDocDir(workPath, docID, workScopes)
	if !found {
		return StatusResponse(ResNotFound, "Not found")
	}
	rootPath := filepath.Join(docDir, "root")
	if st, err := os.Stat(rootPath); err != nil || !st.Mode().IsRegular() {
		return StatusResponse(ResNotFound, "Document not found")
	}
	doc, err := loadWorkDoc(rootPath)
	if err != nil {
		return StatusResponse(ResRemoteFail, "Error loading document")
	}
	meta := docMeta(doc)
	if !equalBytes(metaBytes(meta, "author"), remote.Hash()) {
		return StatusResponse(ResDisallowed, "No access, not author")
	}
	if title != "" {
		meta["title"] = strings.TrimSpace(title)
	}
	if content != "" {
		doc["content"] = strings.TrimSpace(content)
	}
	meta["edited"] = float64(time.Now().UnixNano()) / 1e9
	meta["signature"] = signature
	meta["identity"] = remote.GetPublicKey()
	doc["meta"] = meta
	if err := saveWorkDoc(rootPath, doc); err != nil {
		return StatusResponse(ResRemoteFail, "Error saving document")
	}
	return []byte{ResOK}
}

// workDelete implements Python _work_delete.
func (n *Node) workDelete(workPath, group, repo string, req map[any]any, remote *identity.Identity) any {
	scope := fmt.Sprint(req["scope"])
	if scope == "<nil>" {
		scope = "active"
	}
	if !workScopesValid(scope) {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	v := req["doc_id"]
	if v == nil {
		return StatusResponse(ResInvalidReq, "No document ID specified")
	}
	docID, ok := workDocID(v)
	if !ok {
		return StatusResponse(ResInvalidReq, "Invalid document ID")
	}
	_, docDir, found := workDocDir(workPath, docID, workScopes)
	if !found {
		return StatusResponse(ResNotFound, "Not found")
	}
	rootPath := filepath.Join(docDir, "root")
	if st, err := os.Stat(rootPath); err != nil || !st.Mode().IsRegular() {
		return StatusResponse(ResNotFound, "Document not found")
	}
	doc, err := loadWorkDoc(rootPath)
	if err != nil {
		return StatusResponse(ResRemoteFail, "Error loading document")
	}
	meta := docMeta(doc)
	isAuthor := equalBytes(metaBytes(meta, "author"), remote.Hash())
	adminAccess := n.accessTable().ResolveDoc(group, repo, docID, remote.Hash(), permAdmin)
	if !isAuthor && !adminAccess {
		return StatusResponse(ResDisallowed, "No access, not author")
	}
	// Python unlinks the document .allowed file first and aborts with a
	// remote failure when it is missing, so only documents that carry an
	// ownership file (proposals or perm-edited docs) can be deleted.
	allowedPath := filepath.Join(workPath, strconv.Itoa(docID)+".allowed")
	if err := os.Remove(allowedPath); err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	if err := os.RemoveAll(docDir); err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	return []byte{ResOK}
}

// workComment implements Python _work_comment.
func (n *Node) workComment(workPath string, req map[any]any, remote *identity.Identity) any {
	scope := fmt.Sprint(req["scope"])
	if scope == "<nil>" {
		scope = "active"
	}
	content := strings.TrimSpace(reqString(req, "content"))
	format := reqString(req, "format")
	signature := reqBytes(req, "signature")
	if !workScopesValid(scope) {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	if len(content) > workDocLimit {
		return StatusResponse(ResInvalidReq, "Content limit exceeded")
	}
	v := req["doc_id"]
	if v == nil {
		return StatusResponse(ResInvalidReq, "No document ID specified")
	}
	docID, ok := workDocID(v)
	if !ok {
		return StatusResponse(ResInvalidReq, "Invalid document ID")
	}
	if content == "" {
		return StatusResponse(ResInvalidReq, "Content is required")
	}
	_, docDir, found := workDocDir(workPath, docID, workScopes)
	if !found {
		return StatusResponse(ResNotFound, "Not found")
	}
	rootPath := filepath.Join(docDir, "root")
	if st, err := os.Stat(rootPath); err != nil || !st.Mode().IsRegular() {
		return StatusResponse(ResNotFound, "Document not found")
	}
	commentID := workDirNextID(docDir)
	now := float64(time.Now().UnixNano()) / 1e9
	comment := map[string]any{
		"content": content,
		"meta": map[string]any{
			"format":    workFormat(format),
			"title":     nil,
			"created":   now,
			"edited":    now,
			"signature": signature,
			"author":    remote.Hash(),
		},
	}
	if err := saveWorkDoc(filepath.Join(docDir, strconv.Itoa(commentID)), comment); err != nil {
		return StatusResponse(ResRemoteFail, "Error saving comment")
	}
	return workOK(map[string]any{"id": commentID})
}

// workComplete implements Python _work_complete.
func (n *Node) workComplete(workPath, group, repo string, req map[any]any, remote *identity.Identity) any {
	v := req["doc_id"]
	if v == nil {
		return StatusResponse(ResInvalidReq, "No document ID specified")
	}
	docID, ok := workDocID(v)
	if !ok {
		return StatusResponse(ResInvalidReq, "Invalid document ID")
	}
	activeDir := filepath.Join(workPath, "active", strconv.Itoa(docID))
	if st, err := os.Stat(activeDir); err != nil || !st.IsDir() {
		return StatusResponse(ResNotFound, "Document not found")
	}
	doc, err := loadWorkDoc(filepath.Join(activeDir, "root"))
	if err != nil {
		return StatusResponse(ResRemoteFail, "Error loading document")
	}
	meta := docMeta(doc)
	isAuthor := equalBytes(metaBytes(meta, "author"), remote.Hash())
	adminAccess := n.accessTable().ResolveDoc(group, repo, docID, remote.Hash(), permAdmin)
	if !isAuthor && !adminAccess {
		return StatusResponse(ResDisallowed, "Not allowed")
	}
	completedDir := filepath.Join(workPath, "completed", strconv.Itoa(docID))
	if err := os.MkdirAll(filepath.Dir(completedDir), 0o755); err != nil { // #nosec G301 -- work doc dir
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	if err := os.Rename(activeDir, completedDir); err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	return workOK(map[string]any{"id": docID, "scope": "completed"})
}

// workActivate implements Python _work_activate.
func (n *Node) workActivate(workPath, group, repo string, req map[any]any, remote *identity.Identity) any {
	v := req["doc_id"]
	if v == nil {
		return StatusResponse(ResInvalidReq, "No document ID specified")
	}
	docID, ok := workDocID(v)
	if !ok {
		return StatusResponse(ResInvalidReq, "Invalid document ID")
	}
	_, docDir, found := workDocDir(workPath, docID, []string{"completed", "proposed"})
	if !found {
		return StatusResponse(ResNotFound, "Document not found")
	}
	doc, err := loadWorkDoc(filepath.Join(docDir, "root"))
	if err != nil {
		return StatusResponse(ResRemoteFail, "Error loading document")
	}
	meta := docMeta(doc)
	isAuthor := equalBytes(metaBytes(meta, "author"), remote.Hash())
	adminAccess := n.accessTable().ResolveDoc(group, repo, docID, remote.Hash(), permAdmin)
	if !isAuthor && !adminAccess {
		return StatusResponse(ResDisallowed, "Not allowed")
	}
	activeDir := filepath.Join(workPath, "active", strconv.Itoa(docID))
	if err := os.MkdirAll(filepath.Dir(activeDir), 0o755); err != nil { // #nosec G301 -- work doc dir
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	if err := os.Rename(docDir, activeDir); err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	return workOK(map[string]any{"id": docID, "scope": "active"})
}

// workPerms implements Python _work_perms.
func (n *Node) workPerms(workPath, group, repo string, req map[any]any, remote *identity.Identity) any {
	step := fmt.Sprint(req["step"])
	readAccess := n.accessTable().Resolve(group, repo, remote.Hash(), permRead)
	writeAccess := n.accessTable().Resolve(group, repo, remote.Hash(), permWrite)
	interactAccess := n.accessTable().Resolve(group, repo, remote.Hash(), permInteract)
	manageAccess := interactAccess && writeAccess
	if !readAccess {
		return StatusResponse(ResNotFound, "Not found")
	}
	if !manageAccess {
		return StatusResponse(ResDisallowed, "Not allowed")
	}
	if step == "" || step == "<nil>" {
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
	switch step {
	case "get":
		return n.workGetPermissions(workPath, group, repo, req, remote)
	case "set":
		return n.workSetPermissions(workPath, group, repo, req, remote)
	}
	return StatusResponse(ResInvalidReq, "Invalid step")
}

// workDocPermAccess checks the shared get/set gate of Python
// _work_get_permissions and _work_set_permissions.
func (n *Node) workDocPermAccess(workPath, group, repo string, req map[any]any, remote *identity.Identity) (int, string, []byte) {
	v := req["doc_id"]
	if v == nil {
		return 0, "", StatusResponse(ResInvalidReq, "No document ID specified")
	}
	docID, ok := workDocID(v)
	if !ok {
		return 0, "", StatusResponse(ResInvalidReq, "Invalid document ID")
	}
	_, docDir, found := workDocDir(workPath, docID, workScopes)
	if !found {
		return 0, "", StatusResponse(ResNotFound, "Document not found")
	}
	doc, err := loadWorkDoc(filepath.Join(docDir, "root"))
	if err != nil {
		return 0, "", StatusResponse(ResRemoteFail, "Error loading document")
	}
	meta := docMeta(doc)
	isAuthor := equalBytes(metaBytes(meta, "author"), remote.Hash())
	writeAccess := n.accessTable().Resolve(group, repo, remote.Hash(), permWrite)
	interactAccess := n.accessTable().Resolve(group, repo, remote.Hash(), permInteract)
	adminAccess := n.accessTable().ResolveDoc(group, repo, docID, remote.Hash(), permAdmin)
	manageAccess := interactAccess && writeAccess
	if !(isAuthor && manageAccess) && !adminAccess {
		return 0, "", StatusResponse(ResDisallowed, "Not allowed")
	}
	return docID, docDir, nil
}

// workGetPermissions implements Python _work_get_permissions.
func (n *Node) workGetPermissions(workPath, group, repo string, req map[any]any, remote *identity.Identity) any {
	docID, _, resp := n.workDocPermAccess(workPath, group, repo, req, remote)
	if resp != nil {
		return resp
	}
	content := ""
	if b, err := os.ReadFile(filepath.Join(workPath, strconv.Itoa(docID)+".allowed")); err == nil { // #nosec G304 -- document permission file
		content = string(b)
	}
	return workOK(map[string]any{"content": content})
}

// workSetPermissions implements Python _work_set_permissions.
func (n *Node) workSetPermissions(workPath, group, repo string, req map[any]any, remote *identity.Identity) any {
	docID, _, resp := n.workDocPermAccess(workPath, group, repo, req, remote)
	if resp != nil {
		return resp
	}
	content := reqString(req, "content")
	if invalid, line, ok := ValidateAllowedContent(content, n.accessTable().Aliases); !ok {
		return StatusResponse(ResInvalidReq, fmt.Sprintf("Invalid permission %q on line %d", invalid, line))
	}
	allowedPath := filepath.Join(workPath, strconv.Itoa(docID)+".allowed")
	if err := WriteAllowedFile(allowedPath, content); err != nil {
		return StatusResponse(ResRemoteFail, "Error setting permissions")
	}
	return []byte{ResOK}
}

func reqString(req map[any]any, key string) string {
	v, ok := req[key]
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprint(v)
	}
}

func reqBytes(req map[any]any, key string) []byte {
	v, ok := req[key]
	if !ok || v == nil {
		return nil
	}
	switch b := v.(type) {
	case []byte:
		return b
	case string:
		return []byte(b)
	default:
		return nil
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
