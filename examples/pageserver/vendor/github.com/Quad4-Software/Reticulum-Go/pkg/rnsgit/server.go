// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/node"
	"github.com/Quad4-Software/Reticulum-Go/pkg/rnsutil"
)

// Node hosts git repositories over Reticulum.
type Node struct {
	cfg          *ServerConfig
	access       atomic.Pointer[AccessTable]
	git          *GitRunner
	identity     *identity.Identity
	dest         *destination.Destination
	pageDest     *destination.Destination
	n            *node.Node
	mirrorStop   context.CancelFunc
	announceStop chan struct{}
	mu           sync.RWMutex
	permsMu      sync.Mutex
	workMu       sync.Mutex
	stats        *statsStore
	thanks       thanksTracker
	tmplCache    templateCache
	mdc          *mdToMicron
	hl           *highlighter
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
	nd := &Node{
		cfg:      cfg,
		git:      NewGitRunner(),
		identity: id,
		mdc:      newMdToMicron(100),
		hl:       newHighlighter(),
	}
	nd.access.Store(access)
	return nd, nil
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
		_ = nd.Stop() // #nosec G104 -- rollback partial start
		return err
	}
	n.dest = dest
	n.registerHandlers()
	if n.cfg.AnnounceInterval >= 0 {
		n.announce()
		if n.cfg.AnnounceInterval > 0 {
			n.announceStop = make(chan struct{})
			go n.announceLoop(time.Duration(n.cfg.AnnounceInterval)*time.Minute, n.announceStop)
		}
	}
	if n.cfg.MirrorIntervalHrs > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		n.mirrorStop = cancel
		go n.mirrorScheduler(ctx, time.Duration(n.cfg.MirrorIntervalHrs)*time.Hour)
	}
	if n.cfg.ServeNomadNet {
		if err := n.startPageNode(); err != nil {
			n.logf(debug.DebugError, "Failed to start NomadNet page node", "error", err)
			return err
		}
	}
	n.logf(debug.DebugInfo, "Reticulum Git Node listening", "destination", n.ReposDestHash())
	return nil
}

// Stop shuts down the node.
func (n *Node) Stop() {
	if n.announceStop != nil {
		close(n.announceStop)
		n.announceStop = nil
	}
	if n.mirrorStop != nil {
		n.mirrorStop()
	}
	if n.stats != nil {
		n.stats.close()
	}
	if n.n != nil {
		_ = n.n.Stop() // #nosec G104 -- best effort shutdown
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

// PageDestHash returns the nomadnetwork.node page destination hash hex, or
// the would-be hash when the page node is not running.
func (n *Node) PageDestHash() string {
	if n.pageDest == nil {
		return hex.EncodeToString(destination.Hash(n.identity, pageAppName, pageAppAspect))
	}
	return hex.EncodeToString(n.pageDest.GetHash())
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
	n.announcePages()
}

func (n *Node) announceLoop(interval time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			n.announce()
		}
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
		_ = n.dest.RegisterRequestHandlerAny(h.path, n.wrapHandler(h.path, h.fn), allow, nil)
	}
	n.dest.SetLinkEstablishedCallback(func(_ any) {})
}

// logf emits a node diagnostic at level, honoring the configured loglevel.
func (n *Node) logf(level int, msg string, args ...any) {
	if n.cfg == nil || n.cfg.LogLevel < level {
		return
	}
	debug.Log(level, msg, args...)
}

// logRequest mirrors Python rngit log_request: ordinary requests log at
// verbose while requests from blocked identities log one level deeper.
func (n *Node) logRequest(msg string, remote *identity.Identity) {
	level := debug.DebugVerbose
	if remote != nil && n.cfg.BlockedIdentities[strings.ToLower(hex.EncodeToString(remote.Hash()))] {
		level = debug.DebugTrace
	}
	n.logf(level, msg)
}

// wrapHandler logs each request and any non-OK status result.
func (n *Node) wrapHandler(path string, fn destination.ResponseGeneratorFunc) destination.ResponseGeneratorFunc {
	return func(p string, data []byte, reqID, linkID []byte, remote *identity.Identity, at int64) any {
		remoteStr := "unidentified"
		if remote != nil {
			remoteStr = "<" + hex.EncodeToString(remote.Hash()) + ">"
		}
		n.logRequest(fmt.Sprintf("%s request from remote %s", path, remoteStr), remote)
		out := fn(p, data, reqID, linkID, remote, at)
		if body, ok := out.([]byte); ok && len(body) > 0 && body[0] != ResOK {
			level := debug.DebugVerbose
			if body[0] == ResRemoteFail {
				level = debug.DebugWarning
			}
			n.logf(level, fmt.Sprintf("%s request from remote %s failed", path, remoteStr),
				"code", fmt.Sprintf("0x%02x", body[0]), "detail", string(body[1:]))
		}
		return out
	}
}

// accessTable returns the current permission table. The pointer is swapped
// atomically on reload, so snapshots are safe to walk without a lock.
func (n *Node) accessTable() *AccessTable {
	return n.access.Load()
}

func (n *Node) remoteAllowed(remote *identity.Identity, group, repo string, perm int) bool {
	var hash []byte
	if remote != nil {
		hash = remote.Hash()
	}
	return n.accessTable().Resolve(group, repo, hash, perm)
}

func (n *Node) repoPath(group, repo string) (string, bool) {
	ga, ok := n.accessTable().Groups[group]
	if !ok {
		return "", false
	}
	ra, ok := ga.Repositories[repo]
	if !ok {
		return "", false
	}
	return ra.Path, true
}

// reloadAccess rebuilds the permission table from config and .allowed files,
// matching the Python perms_lock refresh after permission writes.
func (n *Node) reloadAccess() error {
	n.permsMu.Lock()
	defer n.permsMu.Unlock()
	access, err := NewAccessTable(n.cfg)
	if err != nil {
		return err
	}
	n.access.Store(access)
	return nil
}
