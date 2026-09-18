// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/link"
)

// pageTestNode builds a node with one real repository containing a commit.
func pageTestNode(t *testing.T, access string) (*Node, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()

	// Seed repo: work tree with one commit, then a bare clone into the group.
	work := filepath.Join(dir, "seed")
	runGit := func(wd string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...) // #nosec G204 -- test git
		cmd.Dir = wd
		cmd.Env = append(GitCleanEnv(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(work, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(work, "README.md"),
		[]byte("# Demo\n\ntest readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "hello.txt"),
		[]byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(work, "add", ".")
	runGit(work, "-c", "commit.gpgsign=false", "commit", "-m", "initial")

	groupRoot := filepath.Join(dir, "public")
	if err := os.MkdirAll(groupRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(groupRoot, "demo")
	runGit(dir, "clone", "--bare", work, bare)

	cfg := &ServerConfig{
		ConfigDir: dir,
		NodeName:  "Test Node",
		RepositoryGroups: map[string]string{
			"public": groupRoot,
		},
		AccessRules: map[string]string{
			"public": access,
		},
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	node, err := NewNode(cfg, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := node.reloadAccess(); err != nil {
		t.Fatal(err)
	}
	return node, dir
}

func pageReq(t *testing.T, vars map[string]string) []byte {
	t.Helper()
	fields := map[any]any{}
	for k, v := range vars {
		fields["var_"+k] = v
	}
	req, err := EncodeMixedRequest(fields)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func pageBody(t *testing.T, resp any) string {
	t.Helper()
	b, ok := resp.([]byte)
	if !ok {
		t.Fatalf("page response %T not bytes", resp)
	}
	return string(b)
}

func TestFrontPageListsGroup(t *testing.T) {
	node, _ := pageTestNode(t, "rw:all")
	out := pageBody(t, node.serveFrontPage(pagePathIndex, nil, nil, nil, nil, 0))
	if !strings.Contains(out, "public") {
		t.Fatalf("group missing from front page:\n%s", out)
	}
	if !strings.Contains(out, "Test Node") {
		t.Fatalf("node name missing:\n%s", out)
	}
}

func TestGroupPageListsRepo(t *testing.T) {
	node, _ := pageTestNode(t, "rw:all")
	out := pageBody(t, node.serveGroupPage(pagePathGroup,
		pageReq(t, map[string]string{"g": "public"}), nil, nil, nil, 0))
	if !strings.Contains(out, "demo") {
		t.Fatalf("repo missing from group page:\n%s", out)
	}
}

func TestRepoPageCloneURLAndReadme(t *testing.T) {
	node, _ := pageTestNode(t, "rw:all")
	out := pageBody(t, node.serveRepoPage(pagePathRepo,
		pageReq(t, map[string]string{"g": "public", "r": "demo"}), nil, nil, nil, 0))
	if !strings.Contains(out, "rns://") || !strings.Contains(out, "public/demo") {
		t.Fatalf("clone url missing:\n%s", out)
	}
	if !strings.Contains(out, "Demo") {
		t.Fatalf("readme not rendered:\n%s", out)
	}
}

func TestRepoPageNoReadPerm(t *testing.T) {
	node, _ := pageTestNode(t, "")
	out := pageBody(t, node.serveRepoPage(pagePathRepo,
		pageReq(t, map[string]string{"g": "public", "r": "demo"}), nil, nil, nil, 0))
	if strings.Contains(out, "Clone:") {
		t.Fatalf("read-denied repo page rendered content:\n%s", out)
	}
	if !strings.Contains(out, "not found") {
		t.Fatalf("expected not-found error page:\n%s", out)
	}
}

func TestTreePageListsFiles(t *testing.T) {
	node, _ := pageTestNode(t, "rw:all")
	out := pageBody(t, node.serveTreePage(pagePathTree,
		pageReq(t, map[string]string{"g": "public", "r": "demo"}), nil, nil, nil, 0))
	if !strings.Contains(out, "hello.txt") || !strings.Contains(out, "README.md") {
		t.Fatalf("tree entries missing:\n%s", out)
	}
}

func TestBlobPageRendersText(t *testing.T) {
	node, _ := pageTestNode(t, "rw:all")
	out := pageBody(t, node.serveBlobPage(pagePathBlob,
		pageReq(t, map[string]string{
			"g": "public", "r": "demo", "ref": "main", "path": "hello.txt",
		}), nil, nil, nil, 0))
	if !strings.Contains(out, "hello world") {
		t.Fatalf("blob content missing:\n%s", out)
	}
}

func TestBlobPageRejectsTraversal(t *testing.T) {
	node, _ := pageTestNode(t, "rw:all")
	out := pageBody(t, node.serveBlobPage(pagePathBlob,
		pageReq(t, map[string]string{
			"g": "public", "r": "demo", "path": "../../etc/passwd",
		}), nil, nil, nil, 0))
	if strings.Contains(out, "root:") {
		t.Fatalf("traversal served host file:\n%s", out)
	}
}

func TestCommitsPageListsCommit(t *testing.T) {
	node, _ := pageTestNode(t, "rw:all")
	out := pageBody(t, node.serveCommitsPage(pagePathCommits,
		pageReq(t, map[string]string{"g": "public", "r": "demo"}), nil, nil, nil, 0))
	if !strings.Contains(out, "initial") {
		t.Fatalf("commit missing:\n%s", out)
	}
}

func TestRefsPageListsBranch(t *testing.T) {
	node, _ := pageTestNode(t, "rw:all")
	out := pageBody(t, node.serveRefsPage(pagePathRefs,
		pageReq(t, map[string]string{"g": "public", "r": "demo"}), nil, nil, nil, 0))
	if !strings.Contains(out, "main") {
		t.Fatalf("branch missing:\n%s", out)
	}
}

func TestStatsPageDeniedWithoutPerm(t *testing.T) {
	node, _ := pageTestNode(t, "r:all")
	out := pageBody(t, node.serveStatsPage(pagePathStats,
		pageReq(t, map[string]string{"g": "public", "r": "demo"}), nil, nil, nil, 0))
	if strings.Contains(out, "Repository activity") {
		t.Fatalf("stats page rendered without stats perm:\n%s", out)
	}
}

func TestStatsPageWithPerm(t *testing.T) {
	node, _ := pageTestNode(t, "r:all, s:all")
	node.stats = newStatsStore("")
	out := pageBody(t, node.serveStatsPage(pagePathStats,
		pageReq(t, map[string]string{"g": "public", "r": "demo"}), nil, nil, nil, 0))
	if !strings.Contains(out, "activity") && !strings.Contains(out, "Activity") {
		t.Fatalf("stats page empty:\n%s", out)
	}
}

func TestDownloadEndpoint(t *testing.T) {
	node, _ := pageTestNode(t, "rw:all")
	resp := node.serveDownload(filePathDownload,
		pageReq(t, map[string]string{
			"g": "public", "r": "demo", "path": "hello.txt",
		}), nil, nil, nil, 0)
	fr, ok := resp.(link.FileResponse)
	if !ok {
		t.Fatalf("download response %T not a file response", resp)
	}
	if string(fr.Data) != "hello world\n" {
		t.Fatalf("download body: %q", fr.Data)
	}
}
