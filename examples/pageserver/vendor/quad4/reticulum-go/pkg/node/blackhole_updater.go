// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"context"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/blackhole"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
)

const (
	blackholeUpdaterInitialWait = 20 * time.Second
	blackholeUpdaterJobInterval = 60 * time.Second
	blackholeUpdaterDefaultIVL  = 60 * time.Minute
	blackholeListName           = "rnstransport.info.blackhole"
	blackholeUpdateLinkTimeout  = 45 * time.Second
	blackholeRequestTimeout     = 30 * time.Second
	blackholeListPath           = "/list"
)

// StartBlackholeUpdater pulls remote blackhole lists when blackhole_sources
// is configured. Shared-instance clients skip.
func (n *Node) StartBlackholeUpdater() {
	if n == nil || n.config == nil || n.transport == nil {
		return
	}
	if n.transport.ConnectedToSharedInstance() {
		return
	}
	if len(n.config.BlackholeSources) == 0 {
		return
	}
	n.bhUpdaterMu.Lock()
	defer n.bhUpdaterMu.Unlock()
	if n.bhUpdaterRunning {
		return
	}
	n.bhUpdaterRunning = true
	n.bhUpdaterLast = make(map[[16]byte]time.Time)
	n.bhUpdaterStop = make(chan struct{})
	debug.Log(debug.DebugVerbose, "Starting blackhole updater",
		"sources", len(n.config.BlackholeSources))
	go n.blackholeUpdaterLoop()
}

func (n *Node) stopBlackholeUpdater() {
	n.bhUpdaterMu.Lock()
	defer n.bhUpdaterMu.Unlock()
	if !n.bhUpdaterRunning {
		return
	}
	close(n.bhUpdaterStop)
	n.bhUpdaterRunning = false
}

func (n *Node) blackholeUpdateInterval() time.Duration {
	if n == nil || n.config == nil || n.config.BlackholeUpdateInterval <= 0 {
		return blackholeUpdaterDefaultIVL
	}
	return n.config.BlackholeUpdateInterval
}

func (n *Node) blackholeUpdaterLoop() {
	n.bhUpdaterMu.Lock()
	stop := n.bhUpdaterStop
	n.bhUpdaterMu.Unlock()
	select {
	case <-stop:
		return
	case <-time.After(blackholeUpdaterInitialWait):
	}
	ticker := time.NewTicker(blackholeUpdaterJobInterval)
	defer ticker.Stop()
	for {
		n.blackholeUpdaterTick()
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}

func (n *Node) blackholeUpdaterTick() {
	if n == nil || n.config == nil || n.transport == nil {
		return
	}
	interval := n.blackholeUpdateInterval()
	now := time.Now()
	for _, src := range n.config.BlackholeSources {
		if len(src) != 16 {
			continue
		}
		var key [16]byte
		copy(key[:], src)
		n.bhUpdaterMu.Lock()
		last := n.bhUpdaterLast[key]
		n.bhUpdaterMu.Unlock()
		if !last.IsZero() && now.Before(last.Add(interval)) {
			continue
		}
		if err := n.fetchBlackholeFromSource(src); err != nil {
			debug.Log(debug.DebugVerbose, "Blackhole list update failed",
				"source", fmt.Sprintf("%x", src), "error", err)
			continue
		}
		n.bhUpdaterMu.Lock()
		n.bhUpdaterLast[key] = time.Now()
		n.bhUpdaterMu.Unlock()
	}
}

func (n *Node) fetchBlackholeFromSource(sourceHash []byte) error {
	tr := n.transport
	destHash := destination.HashFromNameAndIdentity(blackholeListName, sourceHash)
	ctx, cancel := context.WithTimeout(context.Background(), blackholeUpdateLinkTimeout)
	defer cancel()
	if err := tr.AwaitPath(ctx, destHash); err != nil {
		return fmt.Errorf("path: %w", err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		return fmt.Errorf("recall: %w", err)
	}
	outDest, err := destination.New(remote, destination.Out, destination.Single, "rnstransport", tr, "info", "blackhole")
	if err != nil {
		return err
	}
	established := make(chan struct{}, 1)
	l := link.NewLink(outDest, tr, nil, func(*link.Link) {
		select {
		case established <- struct{}{}:
		default:
		}
	}, nil)
	if err := l.Establish(); err != nil {
		return fmt.Errorf("establish: %w", err)
	}
	select {
	case <-ctx.Done():
		l.Teardown()
		return ctx.Err()
	case <-established:
	case <-time.After(blackholeUpdateLinkTimeout):
		l.Teardown()
		return fmt.Errorf("link establish timeout")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && l.GetStatus() != link.StatusActive {
		time.Sleep(50 * time.Millisecond)
	}
	receipt, err := l.Request(blackholeListPath, nil, blackholeRequestTimeout)
	if err != nil {
		l.Teardown()
		return fmt.Errorf("request: %w", err)
	}
	reqCtx, reqCancel := context.WithTimeout(context.Background(), blackholeRequestTimeout)
	defer reqCancel()
	if err := waitRequestReceipt(reqCtx, receipt); err != nil {
		l.Teardown()
		return err
	}
	l.Teardown()
	if receipt.GetStatus() == link.StatusFailed {
		return fmt.Errorf("request failed")
	}
	raw := receipt.GetResponse()
	if len(raw) == 0 {
		return nil
	}
	decoded, err := blackhole.DecodeBlackholeMap(raw)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	tab := tr.BlackholeTable()
	if tab == nil {
		return fmt.Errorf("no blackhole table")
	}
	if err := tab.MergeRemote(sourceHash, decoded); err != nil {
		return fmt.Errorf("merge: %w", err)
	}
	debug.Log(debug.DebugVerbose, "Blackhole list update completed",
		"source", fmt.Sprintf("%x", sourceHash), "entries", len(decoded))
	return nil
}

func waitRequestReceipt(ctx context.Context, r *link.RequestReceipt) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if r.Concluded() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
