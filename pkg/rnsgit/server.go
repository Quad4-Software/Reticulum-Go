// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
)

// Node hosts git repositories over Reticulum.
type Node struct {
	cfg        *ServerConfig
	access     *AccessTable
	git        *GitRunner
	identity   *identity.Identity
	dest       *destination.Destination
	n          *node.Node
	mirrorStop context.CancelFunc
	mu         sync.RWMutex
}

// NewNode creates a git repository node.
func NewNode(cfg *ServerConfig, id *identity.Identity) (*Node, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	access, err := NewAccessTable(cfg)
	if err != nil {
		return nil, err
	}
	return &Node{
		cfg:      cfg,
		access:   access,
		git:      NewGitRunner(),
		identity: id,
	}, nil
}

// Start runs the repository node.
func (n *Node) Start(rnsConfig string) error {
	rnsCfg, err := rnsutil.LoadConfigDir(rnsConfig)
	if err != nil {
		return err
	}
	nd, err := node.New(rnsCfg)
	if err != nil {
		return err
	}
	if err := nd.Start(); err != nil {
		return err
	}
	n.n = nd
	dest, err := destination.New(n.identity, destination.In, destination.Single, AppName, nd.Transport(), Aspect)
	if err != nil {
		nd.Stop()
		return err
	}
	n.dest = dest
	n.registerHandlers()
	if n.cfg.AnnounceInterval >= 0 {
		n.announce()
		if n.cfg.AnnounceInterval > 0 {
			go n.announceLoop(time.Duration(n.cfg.AnnounceInterval) * time.Minute)
		}
	}
	if n.cfg.MirrorIntervalHrs > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		n.mirrorStop = cancel
		go n.mirrorScheduler(ctx, time.Duration(n.cfg.MirrorIntervalHrs)*time.Hour)
	}
	if n.cfg.ServeNomadNet {
		if err := n.startPageNode(); err != nil {
			return err
		}
	}
	return nil
}

// Stop shuts down the node.
func (n *Node) Stop() {
	if n.mirrorStop != nil {
		n.mirrorStop()
	}
	if n.n != nil {
		n.n.Stop()
	}
}

// ReposDestHash returns the repositories destination hash hex.
func (n *Node) ReposDestHash() string {
	if n.dest == nil {
		return hex.EncodeToString(destination.Hash(n.identity, AppName, Aspect))
	}
	return hex.EncodeToString(n.dest.GetHash())
}

// IdentityHash returns the node identity hash hex.
func (n *Node) IdentityHash() string {
	return hex.EncodeToString(n.identity.Hash())
}

// RunService starts the node until interrupted.
func (n *Node) RunService(rnsConfig string) error {
	if err := n.Start(rnsConfig); err != nil {
		return err
	}
	defer n.Stop()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	return nil
}

func (n *Node) announce() {
	if n.dest != nil {
		_ = n.dest.Announce(false, nil, nil)
	}
}

func (n *Node) announceLoop(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		n.announce()
	}
}

func (n *Node) registerHandlers() {
	allow := byte(destination.AllowAll)
	paths := []struct {
		path string
		fn   destination.ResponseGeneratorFunc
	}{
		{PathList, n.handleList},
		{PathFetch, n.handleFetch},
		{PathPush, n.handlePush},
		{PathDelete, n.handleDelete},
		{PathCreate, n.handleCreate},
		{PathFork, n.handleFork},
		{PathSync, n.handleSync},
		{PathMirror, n.handleMirror},
		{PathPerms, n.handlePerms},
		{PathRelease, n.handleRelease},
		{PathWork, n.handleWork},
	}
	for _, h := range paths {
		_ = n.dest.RegisterRequestHandlerAny(h.path, h.fn, allow, nil)
	}
	n.dest.SetLinkEstablishedCallback(func(_ any) {})
}

func (n *Node) remoteAllowed(remote *identity.Identity, group, repo string, perm int) bool {
	var hash []byte
	if remote != nil {
		hash = remote.Hash()
	}
	return n.access.Resolve(group, repo, hash, perm)
}

func (n *Node) repoPath(group, repo string) (string, bool) {
	ga, ok := n.access.Groups[group]
	if !ok {
		return "", false
	}
	ra, ok := ga.Repositories[repo]
	if !ok {
		return "", false
	}
	return ra.Path, true
}

func (n *Node) reloadAccess() error {
	access, err := NewAccessTable(n.cfg)
	if err != nil {
		return err
	}
	n.mu.Lock()
	n.access = access
	n.mu.Unlock()
	return nil
}
