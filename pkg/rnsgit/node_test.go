// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/identity"
)

func TestNodeListBareRepo(t *testing.T) {
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
			"public": "rw:all",
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
	req, _ := EncodeMixedRequest(map[any]any{IdxRepository: "public/demo"})
	resp := node.handleList(PathList, req, nil, nil, id, 0)
	out, ok := resp.([]byte)
	if !ok || len(out) == 0 || out[0] != ResOK {
		t.Fatalf("list response %T %v", resp, resp)
	}
}
