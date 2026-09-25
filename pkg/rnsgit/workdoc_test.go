// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// newWorkTestNode builds a node with one bare repo and permissive rules.
func newWorkTestNode(t *testing.T, rules string) (*Node, *identity.Identity, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	repoRoot := filepath.Join(dir, "public", "demo")
	if err := os.MkdirAll(filepath.Dir(repoRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	initCmd := exec.Command("git", "init", "--bare", repoRoot) // #nosec G204 -- test git
	initCmd.Env = GitCleanEnv()
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	cfg := &ServerConfig{
		ConfigDir: dir,
		RepositoryGroups: map[string]string{
			"public": filepath.Join(dir, "public"),
		},
		AccessRules: map[string]string{
			"public": rules,
		},
		BlockedIdentities: map[string]bool{},
		IdentityAliases:   map[string]string{},
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	node, err := NewNode(cfg, id)
	if err != nil {
		t.Fatal(err)
	}
	return node, id, dir
}

func workReq(t *testing.T, fields map[any]any) []byte {
	t.Helper()
	if _, ok := fields[IdxRepository]; !ok {
		fields[IdxRepository] = "public/demo"
	}
	req, err := EncodeMixedRequest(fields)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func workResp(t *testing.T, resp any) (byte, []byte) {
	t.Helper()
	out, ok := resp.([]byte)
	if !ok || len(out) == 0 {
		t.Fatalf("response %T %v", resp, resp)
	}
	return out[0], out[1:]
}

func workCreateDoc(t *testing.T, node *Node, id *identity.Identity, op, title, content string) int {
	t.Helper()
	sig, err := id.Sign([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	req := workReq(t, map[any]any{
		"operation": op, "title": title, "content": content,
		"format": "markdown", "signature": sig,
	})
	resp := node.handleWork(PathWork, req, nil, nil, id, 0)
	code, body := workResp(t, resp)
	if code != ResOK {
		t.Fatalf("%s: code %d body %s", op, code, body)
	}
	var result map[string]any
	if err := msgpack.Unmarshal(body, &result); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	docID, ok := workDocID(result["id"])
	if !ok {
		t.Fatalf("no id in %v", result)
	}
	return docID
}

func TestWorkDocLifecycle(t *testing.T) {
	node, id, _ := newWorkTestNode(t, "r:all,rw:all,i:all,p:all,adm:all")

	docID := workCreateDoc(t, node, id, "create", "Task one", "do the thing")

	// list active
	req := workReq(t, map[any]any{"operation": "list", "scope": "active"})
	code, body := workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResOK {
		t.Fatalf("list: %d %s", code, body)
	}
	var list map[string]any
	if err := msgpack.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	active, _ := list["active"].([]any)
	if len(active) != 1 {
		t.Fatalf("active list: %v", list)
	}
	entry, _ := active[0].(map[string]any)
	if entry["title"] != "Task one" {
		t.Fatalf("title: %v", entry)
	}
	if entry["author"] != hexOf(id) {
		t.Fatalf("author: %v", entry["author"])
	}

	// view
	req = workReq(t, map[any]any{"operation": "view", "doc_id": docID, "scope": "all"})
	code, body = workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResOK {
		t.Fatalf("view: %d %s", code, body)
	}
	var doc map[string]any
	if err := msgpack.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["content"] != "do the thing" || doc["scope"] != "active" {
		t.Fatalf("doc: %v", doc)
	}
	meta, _ := doc["meta"].(map[string]any)
	if len(meta["signature"].([]byte)) != 64 {
		t.Fatalf("signature: %v", meta["signature"])
	}
	if len(meta["identity"].([]byte)) != 64 {
		t.Fatalf("identity: %v", meta["identity"])
	}

	// comment
	req = workReq(t, map[any]any{
		"operation": "comment", "doc_id": docID, "scope": "all",
		"content": "first update", "format": "markdown",
	})
	code, body = workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResOK {
		t.Fatalf("comment: %d %s", code, body)
	}
	var cm map[string]any
	if err := msgpack.Unmarshal(body, &cm); err != nil {
		t.Fatal(err)
	}
	if cm["id"] == nil {
		t.Fatalf("comment id missing: %v", cm)
	}

	// edit by author
	newContent := "updated body"
	sig, _ := id.Sign([]byte(newContent))
	req = workReq(t, map[any]any{
		"operation": "edit", "doc_id": docID, "scope": "active",
		"content": newContent, "signature": sig,
	})
	code, _ = workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResOK {
		t.Fatal("edit failed")
	}

	// complete by author
	req = workReq(t, map[any]any{"operation": "complete", "doc_id": docID})
	code, body = workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResOK {
		t.Fatalf("complete: %d %s", code, body)
	}
	var cp map[string]any
	_ = msgpack.Unmarshal(body, &cp)
	if cp["scope"] != "completed" {
		t.Fatalf("scope: %v", cp)
	}

	// activate again
	req = workReq(t, map[any]any{"operation": "activate", "doc_id": docID})
	code, body = workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResOK {
		t.Fatalf("activate: %d %s", code, body)
	}

	// Deleting a created document fails like Python because no .allowed
	// ownership file exists for it.
	req = workReq(t, map[any]any{"operation": "delete", "doc_id": docID, "scope": "active"})
	code, _ = workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResRemoteFail {
		t.Fatalf("delete created doc: code %d", code)
	}

	// Proposed docs carry an .allowed file, so delete succeeds.
	propID := workCreateDoc(t, node, id, "propose", "Prop", "content")
	req = workReq(t, map[any]any{"operation": "delete", "doc_id": propID, "scope": "proposed"})
	code, _ = workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResOK {
		t.Fatal("delete proposed doc failed")
	}
}

func TestWorkDocEditAuthorOnly(t *testing.T) {
	node, id, _ := newWorkTestNode(t, "r:all,rw:all,i:all")
	other, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	docID := workCreateDoc(t, node, id, "create", "Mine", "body")

	content := "hostile takeover"
	sig, _ := other.Sign([]byte(content))
	req := workReq(t, map[any]any{
		"operation": "edit", "doc_id": docID, "content": content, "signature": sig,
	})
	code, body := workResp(t, node.handleWork(PathWork, req, nil, nil, other, 0))
	if code != ResDisallowed {
		t.Fatalf("non-author edit: code %d body %s", code, body)
	}
}

func TestWorkDocAdminCompleteActivate(t *testing.T) {
	// Only admins have interact+write. Author only has read+propose.
	node, author, _ := newWorkTestNode(t, "r:all,p:all")
	admin, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	// Grant admin to a second identity through a repo .allowed file. The
	// repo-level file replaces group fallbacks for listed permissions, so
	// base read and propose access must be restated, matching Python.
	adminHex := hexOf(admin)
	repoAllowed := filepath.Join(node.accessTable().Groups["public"].Path, "demo.allowed")
	if err := WriteAllowedFile(repoAllowed, "adm:"+adminHex+"\nrw:"+adminHex+"\ni:"+adminHex+"\nr:all\n"); err != nil {
		t.Fatal(err)
	}
	if err := node.reloadAccess(); err != nil {
		t.Fatal(err)
	}

	// Author proposes into proposed scope.
	docID := workCreateDoc(t, node, author, "propose", "Proposal", "please review")

	// Author cannot activate own proposal without manage access.
	req := workReq(t, map[any]any{"operation": "activate", "doc_id": docID})
	code, _ := workResp(t, node.handleWork(PathWork, req, nil, nil, author, 0))
	if code != ResDisallowed {
		t.Fatalf("author activate: code %d", code)
	}

	// Admin can activate the proposal.
	code, body := workResp(t, node.handleWork(PathWork, req, nil, nil, admin, 0))
	if code != ResOK {
		t.Fatalf("admin activate: %d %s", code, body)
	}
	var cp map[string]any
	_ = msgpack.Unmarshal(body, &cp)
	if cp["scope"] != "active" {
		t.Fatalf("scope: %v", cp)
	}

	// Admin completes it.
	req = workReq(t, map[any]any{"operation": "complete", "doc_id": docID})
	code, body = workResp(t, node.handleWork(PathWork, req, nil, nil, admin, 0))
	if code != ResOK {
		t.Fatalf("admin complete: %d %s", code, body)
	}
	_ = msgpack.Unmarshal(body, &cp)
	if cp["scope"] != "completed" {
		t.Fatalf("scope: %v", cp)
	}
}

func TestWorkDocBadSignature(t *testing.T) {
	node, id, _ := newWorkTestNode(t, "r:all,rw:all,i:all")
	req := workReq(t, map[any]any{
		"operation": "create", "title": "x", "content": "y",
		"signature": make([]byte, 64),
	})
	code, body := workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResInvalidReq {
		t.Fatalf("code %d body %s", code, body)
	}
}

func TestWorkDocUnidentified(t *testing.T) {
	node, _, _ := newWorkTestNode(t, "r:all")
	req := workReq(t, map[any]any{"operation": "list"})
	code, body := workResp(t, node.handleWork(PathWork, req, nil, nil, nil, 0))
	if code != ResDisallowed || string(body) != "Not identified" {
		t.Fatalf("code %d body %s", code, body)
	}
}

func TestWorkDocPermsGetSet(t *testing.T) {
	node, id, _ := newWorkTestNode(t, "r:all,rw:all,i:all,adm:all")
	docID := workCreateDoc(t, node, id, "create", "Doc", "body")

	req := workReq(t, map[any]any{
		"operation": "perms", "doc_id": docID, "step": "set",
		"content": "i:all\n",
	})
	code, body := workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResOK {
		t.Fatalf("perms set: %d %s", code, body)
	}

	req = workReq(t, map[any]any{
		"operation": "perms", "doc_id": docID, "step": "get",
	})
	code, body = workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResOK {
		t.Fatalf("perms get: %d %s", code, body)
	}
	var result map[string]any
	if err := msgpack.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result["content"] != "i:all\n" {
		t.Fatalf("content: %v", result["content"])
	}

	// Invalid permission content is rejected with a line number.
	req = workReq(t, map[any]any{
		"operation": "perms", "doc_id": docID, "step": "set",
		"content": "bogus line\n",
	})
	code, body = workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResInvalidReq {
		t.Fatalf("invalid perms: %d %s", code, body)
	}
	if !strings.Contains(string(body), "line 1") {
		t.Fatalf("body: %s", body)
	}
}

func TestWorkDocScopedPermissions(t *testing.T) {
	// Base perms allow read+propose only.
	node, proposer, _ := newWorkTestNode(t, "r:all,p:all")
	docID := workCreateDoc(t, node, proposer, "propose", "Proposal", "review me")

	// Proposer's doc .allowed grants i+w to proposer only. A stranger keeps
	// base repo read access but cannot comment without interact.
	stranger, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	req := workReq(t, map[any]any{
		"operation": "comment", "doc_id": docID, "scope": "all", "content": "hi",
	})
	code, _ := workResp(t, node.handleWork(PathWork, req, nil, nil, stranger, 0))
	if code != ResDisallowed {
		t.Fatalf("stranger comment: code %d", code)
	}

	// Author has i+w via doc .allowed so comment works.
	code, body := workResp(t, node.handleWork(PathWork, req, nil, nil, proposer, 0))
	if code != ResOK {
		t.Fatalf("author comment: %d %s", code, body)
	}
}

func TestWorkDocBlockedIdentity(t *testing.T) {
	node, id, _ := newWorkTestNode(t, "r:all,rw:all,i:all")
	node.cfg.BlockedIdentities[hexOf(id)] = true
	if err := node.reloadAccess(); err != nil {
		t.Fatal(err)
	}
	req := workReq(t, map[any]any{"operation": "list"})
	code, _ := workResp(t, node.handleWork(PathWork, req, nil, nil, id, 0))
	if code != ResNotFound {
		t.Fatalf("blocked remote: code %d", code)
	}
}

func hexOf(id *identity.Identity) string {
	return hex.EncodeToString(id.Hash())
}
