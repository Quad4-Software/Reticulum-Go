// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"context"
	"fmt"
	"os"
	"time"
)

// MgmtClient performs remote rngit management commands.
type MgmtClient struct {
	*Client
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
	id, err := PrepareGitIdentity(cfg.IdentityPath)
	if err != nil {
		return nil, err
	}
	tmp, err := osMkdirTemp()
	if err != nil {
		return nil, err
	}
	return &MgmtClient{Client: &Client{
		cfg:          cfg,
		identity:     id,
		refBatchSize: cfg.RefBatchSize,
		tmpDir:       tmp,
	}}, nil
}

func osMkdirTemp() (string, error) {
	return os.MkdirTemp("", "rnsgit-mgmt-")
}

// Connect opens a link to the remote node given an rns:// URL.
func (c *MgmtClient) Connect(ctx context.Context, remote string) error {
	destHex, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	c.destHex = destHex
	if repo != "" {
		c.repoPath = RepoPath(group, repo)
	} else {
		c.repoPath = group
	}
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
	if len(body) == 0 || body[0] != ResOK {
		return fmt.Errorf("create failed: %s", string(body[1:]))
	}
	return nil
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
	if len(body) == 0 || body[0] != ResOK {
		return fmt.Errorf("sync failed: %s", string(body[1:]))
	}
	return nil
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
	if len(body) == 0 || body[0] != ResOK {
		return fmt.Errorf("%s failed: %s", kind, string(body[1:]))
	}
	return nil
}
