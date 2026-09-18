// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// releaseRules grants read and release access to all identities.
const releaseRules = "r:all\nrel:all\nadm:all\n"

// seedRepoCommit writes a blob, tree, and commit to a bare repo and
// returns the commit sha.
func seedRepoCommit(t *testing.T, repoPath string) string {
	t.Helper()
	env := append(GitCleanEnv(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	run := func(stdin string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...) // #nosec G204 -- test git
		cmd.Env = env
		if stdin != "" {
			cmd.Stdin = strings.NewReader(stdin)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	blob := run("hello", "hash-object", "-w", "--stdin")
	tree := run("100644 blob "+blob+"\tfile.txt\n", "mktree")
	commit := run("", "commit-tree", tree, "-m", "init")
	run("", "update-ref", "refs/heads/master", commit)
	return commit
}

func TestReleaseLifecycle(t *testing.T) {
	node, id, _ := newWorkTestNode(t, releaseRules)
	repoPath, ok := node.repoPath("public", "demo")
	if !ok {
		t.Fatal("repo path")
	}
	sha := seedRepoCommit(t, repoPath)
	if _, stderr, err := node.git.run(repoPath, "tag", "v1.0", sha); err != nil {
		t.Fatalf("tag: %v %s", err, stderr)
	}

	// init
	resp := node.handleRelease("", workReq(t, map[any]any{
		"operation": "create", "step": "init",
		"tag": "v1.0", "notes": "# Heading\n\nFirst release notes\n",
		"hash": sha,
	}), nil, nil, id, 0)
	code, _ := workResp(t, resp)
	if code != ResOK {
		t.Fatalf("init failed: %s", resp.([]byte)[1:])
	}
	releasesPath := repoPath + ".releases"
	meta, err := readReleaseMeta(filepath.Join(releasesPath, "v1.0", "META"))
	if err != nil {
		t.Fatalf("META: %v", err)
	}
	if meta["status"] != "draft" || meta["tag"] != "v1.0" {
		t.Fatalf("meta: %v", meta)
	}

	// artifact upload only while draft
	resp = node.handleRelease("", workReq(t, map[any]any{
		"operation": "create", "step": "artifact",
		"tag": "v1.0", "artifact_name": "app.bin", "artifact_data": []byte{1, 2, 3},
	}), nil, nil, id, 0)
	code, _ = workResp(t, resp)
	if code != ResOK {
		t.Fatalf("artifact: %s", resp.([]byte)[1:])
	}
	if b, err := os.ReadFile(filepath.Join(releasesPath, "v1.0", "artifacts", "app.bin")); err != nil || len(b) != 3 {
		t.Fatalf("artifact file: %v", err)
	}

	// list shows the draft release
	resp = node.handleRelease("", workReq(t, map[any]any{"operation": "list"}), nil, nil, id, 0)
	code, body := workResp(t, resp)
	if code != ResOK {
		t.Fatalf("list: %s", body)
	}
	var listed map[string]any
	if err := msgpack.Unmarshal(body, &listed); err != nil {
		t.Fatalf("list decode: %v", err)
	}
	if rels, ok := listed["releases"].([]any); !ok || len(rels) != 1 {
		t.Fatalf("releases: %#v", listed)
	}
	if latest := listed["latest"]; latest != nil {
		t.Fatalf("latest should be nil before publish: %v", latest)
	}

	// finalize publishes and marks latest
	resp = node.handleRelease("", workReq(t, map[any]any{
		"operation": "create", "step": "finalize", "tag": "v1.0",
	}), nil, nil, id, 0)
	code, _ = workResp(t, resp)
	if code != ResOK {
		t.Fatalf("finalize: %s", resp.([]byte)[1:])
	}
	meta, _ = readReleaseMeta(filepath.Join(releasesPath, "v1.0", "META"))
	if meta["status"] != "published" || meta["published_at"] == "" {
		t.Fatalf("published meta: %v", meta)
	}
	if b, _ := os.ReadFile(filepath.Join(releasesPath, "latest")); string(b) != "v1.0" {
		t.Fatalf("latest marker: %q", b)
	}

	// artifact upload after finalize must be refused
	resp = node.handleRelease("", workReq(t, map[any]any{
		"operation": "create", "step": "artifact",
		"tag": "v1.0", "artifact_name": "late.bin", "artifact_data": []byte{9},
	}), nil, nil, id, 0)
	code, _ = workResp(t, resp)
	if code != ResDisallowed {
		t.Fatalf("post-finalize artifact: %s", resp.([]byte)[1:])
	}

	// view returns notes, artifacts and meta
	resp = node.handleRelease("", workReq(t, map[any]any{
		"operation": "view", "tag": "latest",
	}), nil, nil, id, 0)
	code, body = workResp(t, resp)
	if code != ResOK {
		t.Fatalf("view: %s", body)
	}
	var view map[string]any
	if err := msgpack.Unmarshal(body, &view); err != nil {
		t.Fatalf("view decode: %v", err)
	}
	if view["status"] != "published" || view["tag"] != "v1.0" {
		t.Fatalf("view: %#v", view)
	}
	notes, _ := view["notes"].(string)
	if notes == "" || view["notes_format"] != "markdown" {
		t.Fatalf("notes: %#v", view["notes"])
	}
	if arts, ok := view["artifacts"].([]any); !ok || len(arts) != 1 {
		t.Fatalf("artifacts: %#v", view["artifacts"])
	}

	// fetch returns the artifact file with name metadata
	resp = node.handleRelease("", workReq(t, map[any]any{
		"operation": "fetch", "tag": "v1.0", "artifact": "app.bin",
	}), nil, nil, id, 0)
	fr, ok := resp.(link.FileResponse)
	if !ok {
		t.Fatalf("fetch response %T", resp)
	}
	if len(fr.Data) != 3 {
		t.Fatalf("artifact data: %v", fr.Data)
	}
	var metaMap map[string]any
	if err := msgpack.Unmarshal(fr.MetadataPacked, &metaMap); err != nil {
		t.Fatalf("meta: %v", err)
	}
	if name, _ := metaMap["name"].([]byte); string(name) != "app.bin" {
		t.Fatalf("name meta: %#v", metaMap)
	}

	// delete removes the release
	resp = node.handleRelease("", workReq(t, map[any]any{
		"operation": "delete", "tag": "v1.0",
	}), nil, nil, id, 0)
	code, _ = workResp(t, resp)
	if code != ResOK {
		t.Fatalf("delete: %s", resp.([]byte)[1:])
	}
	if _, err := os.Stat(filepath.Join(releasesPath, "v1.0")); !os.IsNotExist(err) {
		t.Fatalf("release dir still present")
	}
}

func TestReleaseAccessGates(t *testing.T) {
	node, id, _ := newWorkTestNode(t, "r:all\n")
	// read-only identity can list but not create
	resp := node.handleRelease("", workReq(t, map[any]any{"operation": "list"}), nil, nil, id, 0)
	code, _ := workResp(t, resp)
	if code != ResOK {
		t.Fatalf("list: %v", resp)
	}
	resp = node.handleRelease("", workReq(t, map[any]any{
		"operation": "create", "step": "init", "tag": "v9",
	}), nil, nil, id, 0)
	code, _ = workResp(t, resp)
	if code != ResDisallowed {
		t.Fatalf("create without rel perm: code %d", code)
	}
	// unknown op
	resp = node.handleRelease("", workReq(t, map[any]any{"operation": "bogus"}), nil, nil, id, 0)
	code, _ = workResp(t, resp)
	if code != ResDisallowed {
		t.Fatalf("bogus op: code %d", code)
	}
	// unidentified remote
	resp = node.handleRelease("", workReq(t, map[any]any{"operation": "list"}), nil, nil, nil, 0)
	code, _ = workResp(t, resp)
	if code != ResDisallowed {
		t.Fatalf("nil remote: code %d", code)
	}
	// missing repository field
	req, _ := EncodeMixedRequest(map[any]any{"operation": "list"})
	resp = node.handleRelease("", req, nil, nil, id, 0)
	code, body := workResp(t, resp)
	if code != ResInvalidReq || string(body) != "No repository specified" {
		t.Fatalf("missing repo: %d %s", code, body)
	}
}
