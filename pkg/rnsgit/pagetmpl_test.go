// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

func testPageNode(t *testing.T) *Node {
	t.Helper()
	dir := t.TempDir()
	cfg := &ServerConfig{
		ConfigDir: dir,
		NodeName:  "Test Node",
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	n, err := NewNode(cfg, id)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDefaultTemplatesPresent(t *testing.T) {
	for _, name := range []string{
		"base", "front", "group", "repo", "tree", "blob", "commits",
		"commit", "refs", "stats", "releases", "release", "work", "work_doc",
	} {
		if defaultTemplates[name] == "" {
			t.Fatalf("missing default template %q", name)
		}
		if !strings.Contains(defaultTemplates[name], "{PAGE_CONTENT}") {
			t.Fatalf("template %q lacks PAGE_CONTENT marker", name)
		}
	}
}

func TestRenderTemplateSubstitutions(t *testing.T) {
	n := testPageNode(t)
	out := string(n.renderTemplate("BODY", "NAV", "front", time.Now()))
	for _, want := range []string{"BODY", "NAV", "Test Node", Version} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered page missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "{PAGE_CONTENT}") || strings.Contains(out, "{NODE_NAME}") {
		t.Fatalf("unreplaced marker in output:\n%s", out)
	}
}

func TestTemplateOverrideFile(t *testing.T) {
	n := testPageNode(t)
	dir := filepath.Join(n.cfg.ConfigDir, "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "OVERRIDE {PAGE_CONTENT} END"
	if err := os.WriteFile(filepath.Join(dir, "front.mu"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	out := string(n.renderTemplate("INNER", "", "front", time.Now()))
	if !strings.Contains(out, "OVERRIDE INNER END") {
		t.Fatalf("override not applied: %q", out)
	}
}

func TestTemplateOverrideCacheInvalidatesOnMtime(t *testing.T) {
	n := testPageNode(t)
	dir := filepath.Join(n.cfg.ConfigDir, "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "front.mu")
	if err := os.WriteFile(path, []byte("V1 {PAGE_CONTENT}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := n.getTemplate("front"); !strings.Contains(out, "V1") {
		t.Fatalf("first template not loaded: %q", out)
	}
	// Force an older cached mtime then bump the file.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("V2 {PAGE_CONTENT}"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
	if out := n.getTemplate("front"); !strings.Contains(out, "V2") {
		t.Fatalf("template cache did not invalidate: %q", out)
	}
}

func TestExecutableTemplateBounded(t *testing.T) {
	n := testPageNode(t)
	dir := filepath.Join(n.cfg.ConfigDir, "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "front.mu")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	out := n.getTemplate("front")
	if elapsed := time.Since(start); elapsed > 12*time.Second {
		t.Fatalf("executable template not bounded, took %v", elapsed)
	}
	_ = out // timed-out output falls back to empty or default
}

func TestPageVarsDecoding(t *testing.T) {
	req, err := EncodeMixedRequest(map[any]any{
		"var_g":   "public",
		"var_r":   "demo",
		"var_ref": "main",
		"other":   "ignored",
	})
	if err != nil {
		t.Fatal(err)
	}
	vars := pageVars(req)
	if vars["g"] != "public" || vars["r"] != "demo" || vars["ref"] != "main" {
		t.Fatalf("vars: %v", vars)
	}
	if _, ok := vars["other"]; ok {
		t.Fatal("non-var key leaked into page vars")
	}
}

func TestVarInt(t *testing.T) {
	vars := map[string]string{"page": "3", "bad": "x"}
	if got := varInt(vars, "page", 0); got != 3 {
		t.Fatalf("varInt page: %d", got)
	}
	if got := varInt(vars, "bad", 7); got != 7 {
		t.Fatalf("varInt bad: %d", got)
	}
	if got := varInt(vars, "missing", 9); got != 9 {
		t.Fatalf("varInt missing: %d", got)
	}
}
