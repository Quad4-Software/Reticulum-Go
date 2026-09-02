// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// MgmtClient performs remote rngit management commands.
type MgmtClient struct {
	*Client
	status io.Writer
}

// NewMgmtClient creates a management client.
func NewMgmtClient(configDir, rnsConfig string) (*MgmtClient, error) {
	if err := EnsureClientConfig(configDir); err != nil {
		return nil, err
	}
	cfg, err := LoadClientConfig(configDir)
	if err != nil {
		return nil, err
	}
	if rnsConfig != "" {
		cfg.RNSConfigDir = rnsConfig
	}
	id, err := PrepareGitIdentity(cfg.IdentityPath)
	if err != nil {
		return nil, err
	}
	tmp, err := osMkdirTemp()
	if err != nil {
		return nil, err
	}
	batch := cfg.RefBatchSize
	if batch <= 0 {
		batch = DefaultRefBatchSize
	}
	return &MgmtClient{Client: &Client{
		cfg:          cfg,
		identity:     id,
		refBatchSize: batch,
		tmpDir:       tmp,
	}}, nil
}

func osMkdirTemp() (string, error) {
	return os.MkdirTemp("", "rnsgit-mgmt-")
}

// SetStatusWriter sets optional progress lines for connect and mgmt commands.
func (c *MgmtClient) SetStatusWriter(w io.Writer) {
	c.status = w
}

// Connect opens a link to the remote node given an rns:// URL.
func (c *MgmtClient) Connect(ctx context.Context, remote string) error {
	destHex, group, repo, err := ParseRNSURL(strings.TrimSuffix(remote, "/"))
	if err != nil {
		return err
	}
	c.destHex = destHex
	if repo != "" {
		c.repoPath = RepoPath(group, repo)
	} else {
		c.repoPath = group
	}
	c.progressWriter = c.status
	return c.Client.Connect(ctx)
}

// CreateRepo requests repository creation on the remote node.
func (c *MgmtClient) CreateRepo(ctx context.Context, remote string) error {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	req, err := EncodeRequest(map[int]any{IdxRepository: RepoPath(group, repo)})
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, PathCreate, req, 120*time.Second)
	if err != nil {
		return err
	}
	return checkStatus(body, "create")
}

// SyncRepo requests upstream sync for a fork or mirror.
func (c *MgmtClient) SyncRepo(ctx context.Context, remote string) error {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	req, err := EncodeRequest(map[int]any{IdxRepository: RepoPath(group, repo)})
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, PathSync, req, 2*time.Hour)
	if err != nil {
		return err
	}
	return checkStatus(body, "sync")
}

// CloneRemote forks or mirrors a repository on the remote node.
func (c *MgmtClient) CloneRemote(ctx context.Context, source, target, kind string) error {
	_, group, repo, err := ParseRNSURL(target)
	if err != nil {
		return err
	}
	path := PathFork
	if kind == "mirror" {
		path = PathMirror
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"source":      source,
	})
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, path, req, 2*time.Hour)
	if err != nil {
		return err
	}
	return checkStatus(body, kind)
}

// ManagePermissions reads and optionally writes group or repository permissions.
func (c *MgmtClient) ManagePermissions(ctx context.Context, remote, contentPath string, useEditor bool) error {
	remote = strings.TrimSuffix(remote, "/")
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	op := "gperms"
	getReq := map[any]any{"operation": op, "step": "get", IdxGroup: group}
	setReqKey := any(IdxGroup)
	setReqVal := any(group)
	if repo != "" {
		op = "rperms"
		repoPath := RepoPath(group, repo)
		getReq = map[any]any{"operation": op, "step": "get", IdxRepository: repoPath}
		setReqKey = IdxRepository
		setReqVal = repoPath
	}
	body, err := c.sendRequest(ctx, PathPerms, getReq, 120*time.Second)
	if err != nil {
		return err
	}
	current, err := ParsePermsGetBody(body)
	if err != nil {
		return err
	}
	var content string
	switch {
	case contentPath != "":
		b, err := os.ReadFile(contentPath) // #nosec G304 -- operator-chosen permissions file
		if err != nil {
			return err
		}
		content = string(b)
	case useEditor:
		edited, ok, err := editWithEditor(current)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("edit cancelled")
		}
		content = edited
	default:
		return fmt.Errorf("provide -content or set EDITOR to modify permissions")
	}
	setReq := map[any]any{"operation": op, "step": "set", "content": content, setReqKey: setReqVal}
	body, err = c.sendRequest(ctx, PathPerms, setReq, 120*time.Second)
	if err != nil {
		return err
	}
	return checkStatus(body, "permissions")
}

// ReleaseList returns the release table text from the remote repository.
func (c *MgmtClient) ReleaseList(ctx context.Context, remote string) (string, error) {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return "", err
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   "list",
	})
	if err != nil {
		return "", err
	}
	body, err := c.sendRequest(ctx, PathRelease, req, 120*time.Second)
	if err != nil {
		return "", err
	}
	return stringBody(body, "list releases")
}

// ReleaseFetch downloads a release artifact to outPath.
func (c *MgmtClient) ReleaseFetch(ctx context.Context, remote, tag, artifact, outPath string) error {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   "fetch",
		"tag":         tag,
		"artifact":    artifact,
	})
	if err != nil {
		return err
	}
	body, meta, err := c.sendRequestWithMeta(ctx, PathRelease, req, 2*time.Hour)
	if err != nil {
		return err
	}
	if meta != nil {
		if code, ok := MetadataResultCode(meta); ok && code != ResOK {
			return fmt.Errorf("fetch release failed")
		}
	} else if len(body) == 0 || body[0] != ResOK {
		return fmt.Errorf("fetch release failed: %s", string(body[1:]))
	}
	if len(body) == 0 {
		return fmt.Errorf("empty release artifact")
	}
	if outPath == "" {
		outPath = artifact
	}
	if err := os.WriteFile(outPath, body, 0o644); err != nil { // #nosec G306 -- downloaded artifact
		return err
	}
	return nil
}

// ReleaseCreate publishes a release tag with notes on the remote repository.
func (c *MgmtClient) ReleaseCreate(ctx context.Context, remote, tag, notes string) error {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   "create",
		"tag":         tag,
		"notes":       notes,
	})
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, PathRelease, req, 120*time.Second)
	if err != nil {
		return err
	}
	return checkStatus(body, "create release")
}

// WorkList returns work document listing text from the remote group.
func (c *MgmtClient) WorkList(ctx context.Context, remote string) (string, error) {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return "", err
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   "list",
	})
	if err != nil {
		return "", err
	}
	body, err := c.sendRequest(ctx, PathWork, req, 120*time.Second)
	if err != nil {
		return "", err
	}
	return stringBody(body, "list work")
}

// WorkView returns a work document body.
func (c *MgmtClient) WorkView(ctx context.Context, remote, docID string) (string, error) {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return "", err
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   "view",
		"doc_id":      docID,
	})
	if err != nil {
		return "", err
	}
	body, err := c.sendRequest(ctx, PathWork, req, 120*time.Second)
	if err != nil {
		return "", err
	}
	return stringBody(body, "view work")
}

// WorkCreate opens an editor and creates a work document on the remote repository.
func (c *MgmtClient) WorkCreate(ctx context.Context, remote, title, contentPath string) (string, error) {
	content, err := c.workContent(title, contentPath)
	if err != nil {
		return "", err
	}
	return c.workSubmit(ctx, remote, "create", title, content)
}

// WorkPropose opens an editor and proposes a work document on the remote repository.
func (c *MgmtClient) WorkPropose(ctx context.Context, remote, title, contentPath string) (string, error) {
	content, err := c.workContent(title, contentPath)
	if err != nil {
		return "", err
	}
	return c.workSubmit(ctx, remote, "propose", title, content)
}

func (c *MgmtClient) workContent(title, contentPath string) (string, error) {
	if contentPath != "" {
		b, err := os.ReadFile(contentPath) // #nosec G304 -- operator-chosen work doc
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	initial := title + "\n\n"
	edited, ok, err := editWithEditor(initial)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("edit cancelled")
	}
	return edited, nil
}

func (c *MgmtClient) workSubmit(ctx context.Context, remote, operation, title, content string) (string, error) {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return "", err
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   operation,
		"title":       title,
		"content":     content,
	})
	if err != nil {
		return "", err
	}
	body, err := c.sendRequest(ctx, PathWork, req, 120*time.Second)
	if err != nil {
		return "", err
	}
	if len(body) == 0 || body[0] != ResOK {
		return "", fmt.Errorf("%s failed: %s", operation, string(body[1:]))
	}
	if len(body) > 1 {
		return strings.TrimSpace(string(body[1:])), nil
	}
	return "", nil
}

func checkStatus(body []byte, op string) error {
	if len(body) == 0 || body[0] != ResOK {
		msg := string(body[1:])
		if msg == "" {
			msg = op + " failed"
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func stringBody(body []byte, op string) (string, error) {
	if len(body) == 0 || body[0] != ResOK {
		msg := string(body[1:])
		if msg == "" {
			msg = op + " failed"
		}
		return "", fmt.Errorf("%s", msg)
	}
	return string(body[1:]), nil
}
