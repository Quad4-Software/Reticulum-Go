// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	return g.runCtx(context.Background(), dir, args...)
}

func (g *GitRunner) runCtx(ctx context.Context, dir string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, g.Git, args...) // #nosec G204 -- fixed git binary
	cmd.Dir = dir
	cmd.Env = envWithoutGitDir()
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
	if b, err := os.ReadFile(filepath.Join(repoPath, "HEAD")); err == nil { // #nosec G304 -- bare repo HEAD
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
	if err := os.MkdirAll(path, 0o755); err != nil { // #nosec G301 -- bare git repo root
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- bare git parent dir
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

// RevParseVerify resolves a ref to a sha, matching Python resolve_ref.
func (g *GitRunner) RevParseVerify(repoPath, ref string) (string, error) {
	if strings.HasPrefix(ref, "-") || ref == "" {
		return "", fmt.Errorf("invalid ref")
	}
	out, stderr, err := g.run(repoPath, "rev-parse", "--verify", ref)
	if err != nil {
		return "", fmt.Errorf("git rev-parse --verify: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return strings.ToLower(strings.TrimSpace(string(out))), nil
}

// BlobSize returns the size of a blob at ref:path, matching Python
// get_blob_info size resolution.
func (g *GitRunner) BlobSize(repoPath, ref, filePath string) (int64, error) {
	out, stderr, err := g.run(repoPath, "cat-file", "-s", ref+":"+strings.Trim(filePath, "/"))
	if err != nil {
		return 0, fmt.Errorf("git cat-file: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("git cat-file size: %w", err)
	}
	return size, nil
}

// BlobContent returns blob bytes at ref:path, matching Python
// get_blob_content.
func (g *GitRunner) BlobContent(repoPath, ref, filePath string) ([]byte, error) {
	out, stderr, err := g.run(repoPath, "show", ref+":"+strings.Trim(filePath, "/"))
	if err != nil {
		return nil, fmt.Errorf("git show: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return out, nil
}

// LsRemoteHead returns the symref target branch for HEAD on a remote
// source, or an empty string if it cannot be determined. Matches the
// ls-remote --symref step of Python __update_head_to_source_default.
func (g *GitRunner) LsRemoteHead(source string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, _, err := g.runCtx(ctx, "", "ls-remote", "--symref", source, "HEAD")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "ref: refs/heads/") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 && parts[1] == "HEAD" {
			return strings.TrimSpace(parts[0][5:])
		}
	}
	return ""
}

// ShowRefVerify reports whether ref resolves, matching
// git show-ref --verify --quiet.
func (g *GitRunner) ShowRefVerify(repoPath, ref string) bool {
	_, _, err := g.run(repoPath, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

// FirstLocalBranch returns the first branch under refs/heads, or an
// empty string when the repository has no branches.
func (g *GitRunner) FirstLocalBranch(repoPath string) string {
	out, _, err := g.run(repoPath, "for-each-ref", "--format=%(refname:short)", "refs/heads", "--count=1")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SetHead points HEAD at ref via git symbolic-ref.
func (g *GitRunner) SetHead(repoPath, ref string) error {
	_, stderr, err := g.run(repoPath, "symbolic-ref", "HEAD", ref)
	if err != nil {
		return fmt.Errorf("git symbolic-ref: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// UpdateHeadToSourceDefault points HEAD at the remote's default branch,
// falling back to the first local branch. Matches Python
// __update_head_to_source_default. Best effort, like the Python caller.
func (g *GitRunner) UpdateHeadToSourceDefault(repoPath, source string) {
	target := g.LsRemoteHead(source)
	if target != "" && !g.ShowRefVerify(repoPath, target) {
		target = ""
	}
	if target == "" {
		name := g.FirstLocalBranch(repoPath)
		if name == "" {
			return
		}
		target = "refs/heads/" + name
	}
	_ = g.SetHead(repoPath, target)
}
