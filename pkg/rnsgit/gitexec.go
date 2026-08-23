// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitRunner executes git subprocesses in a repository directory.
type GitRunner struct {
	Git string
}

// NewGitRunner returns a runner using git from PATH.
func NewGitRunner() *GitRunner {
	return &GitRunner{Git: "git"}
}

func (g *GitRunner) run(dir string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.Command(g.Git, args...) // #nosec G204 -- fixed git binary
	cmd.Dir = dir
	cmd.Env = gitCleanEnv()
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

// ListRefs returns git for-each-ref output lines.
func (g *GitRunner) ListRefs(repoPath string) (string, error) {
	out, stderr, err := g.run(repoPath, "for-each-ref", "--format", "%(objectname) %(refname)")
	if err != nil {
		return "", fmt.Errorf("git for-each-ref: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	headRef := "master"
	if b, err := os.ReadFile(filepath.Join(repoPath, "HEAD")); err == nil {
		s := strings.TrimSpace(string(b))
		if after, ok := strings.CutPrefix(s, "ref: "); ok {
			headRef = after
		}
	}
	body := strings.TrimSpace(string(out))
	if body != "" {
		body += "\n"
	}
	body += fmt.Sprintf("@%s HEAD\n", headRef)
	return body, nil
}

// CreateBundle builds a fetch bundle for refs.
func (g *GitRunner) CreateBundle(repoPath, bundlePath string, refs []map[string]string, have []string) error {
	args := []string{"bundle", "create", "--no-progress", bundlePath}
	for _, r := range refs {
		ref := r["ref"]
		if ref == "" {
			continue
		}
		args = append(args, ref)
		if haveLocal, ok := r["have"]; ok && SanSHA(haveLocal) != "" {
			args = append(args, "^"+haveLocal)
		}
	}
	for _, sha := range have {
		if SanSHA(sha) == "" {
			continue
		}
		if _, _, err := g.run(repoPath, "cat-file", "-t", sha); err == nil {
			args = append(args, "^"+sha)
		}
	}
	_, stderr, err := g.run(repoPath, args...)
	if err != nil {
		if strings.Contains(strings.ToLower(string(stderr)), "empty bundle") {
			return errEmptyBundle
		}
		return fmt.Errorf("git bundle create: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

var errEmptyBundle = fmt.Errorf("empty bundle")

// IsEmptyBundle reports whether err indicates an empty bundle.
func IsEmptyBundle(err error) bool {
	return err != nil && strings.Contains(err.Error(), "empty bundle")
}

// VerifyBundle checks a bundle file.
func (g *GitRunner) VerifyBundle(repoPath, bundlePath string) error {
	_, stderr, err := g.run(repoPath, "bundle", "verify", bundlePath)
	if err != nil {
		return fmt.Errorf("git bundle verify: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// FetchBundle imports a bundle refspec.
func (g *GitRunner) FetchBundle(repoPath, bundlePath, localRef, remoteRef string, force bool) error {
	spec := localRef + ":" + remoteRef
	args := []string{"fetch", bundlePath, spec}
	if force {
		args = append(args, "--force")
	}
	_, stderr, err := g.run(repoPath, args...)
	if err != nil {
		return fmt.Errorf("git fetch bundle: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// UpdateRef sets a ref to a sha.
func (g *GitRunner) UpdateRef(repoPath, ref, sha string, force bool) error {
	if SanRef(ref) == "" || SanSHA(sha) == "" {
		return fmt.Errorf("invalid ref or sha")
	}
	if !force {
		if out, _, err := g.run(repoPath, "rev-parse", ref); err == nil {
			existing := strings.TrimSpace(string(out))
			if existing != "" && existing != sha {
				return fmt.Errorf("ref exists at different sha")
			}
		}
	}
	_, stderr, err := g.run(repoPath, "update-ref", ref, sha)
	if err != nil {
		return fmt.Errorf("git update-ref: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// DeleteRef removes a ref.
func (g *GitRunner) DeleteRef(repoPath, ref string) error {
	_, stderr, err := g.run(repoPath, "update-ref", "-d", ref)
	if err != nil {
		return fmt.Errorf("git update-ref -d: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// InitBare creates a bare repository.
func (g *GitRunner) InitBare(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	_, stderr, err := g.run(path, "init", "--bare")
	if err != nil {
		return fmt.Errorf("git init --bare: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// CloneBare mirrors a remote into path.
func (g *GitRunner) CloneBare(source, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, stderr, err := g.run("", "clone", "--bare", source, path)
	if err != nil {
		return fmt.Errorf("git clone --bare: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// FetchAll updates a bare mirror from source URL.
func (g *GitRunner) FetchAll(repoPath, source string) error {
	_, stderr, err := g.run(repoPath, "fetch", source, "+refs/*:refs/*")
	if err != nil {
		return fmt.Errorf("git fetch: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// SetConfig sets a repository config value.
func (g *GitRunner) SetConfig(repoPath, key, value string) error {
	_, stderr, err := g.run(repoPath, "config", key, value)
	if err != nil {
		return fmt.Errorf("git config: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// ConfigValue reads a git config value from a repository.
func (g *GitRunner) ConfigValue(repoPath, key string) (string, error) {
	out, stderr, err := g.run(repoPath, "config", "--get", key)
	if err != nil {
		return "", fmt.Errorf("git config --get: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return strings.TrimSpace(string(out)), nil
}

// ObjectExists checks whether sha exists in repo.
func (g *GitRunner) ObjectExists(repoPath, sha string) bool {
	_, _, err := g.run(repoPath, "cat-file", "-t", sha)
	return err == nil
}

// RevParse resolves a ref to sha.
func (g *GitRunner) RevParse(repoPath, ref string) (string, error) {
	out, stderr, err := g.run(repoPath, "rev-parse", ref)
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return strings.TrimSpace(string(out)), nil
}
