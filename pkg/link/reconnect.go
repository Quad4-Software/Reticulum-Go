// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"quad4/reticulum-go/pkg/transport"
)

var globalPaused atomic.Bool

var (
	reconnectCancelsMu sync.Mutex
	reconnectCancels   = map[uintptr]context.CancelFunc{}
	reconnectSeq       atomic.Uint64
)

// SetGlobalPaused freezes link watchdog stale timers during known sleep.
// When paused, in-flight reconnect loops are cancelled so they do not keep
// hammering Reestablish until I/O fails.
func SetGlobalPaused(paused bool) {
	globalPaused.Store(paused)
	if paused {
		CancelAllReconnects()
	}
}

// GlobalPaused reports whether link watchdog timers are frozen.
func GlobalPaused() bool {
	return globalPaused.Load()
}

// CancelAllReconnects aborts every WatchAndReconnect reestablish loop.
func CancelAllReconnects() {
	reconnectCancelsMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(reconnectCancels))
	for id, cancel := range reconnectCancels {
		cancels = append(cancels, cancel)
		delete(reconnectCancels, id)
	}
	reconnectCancelsMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func registerReconnectCancel(cancel context.CancelFunc) uint64 {
	id := reconnectSeq.Add(1)
	reconnectCancelsMu.Lock()
	reconnectCancels[uintptr(id)] = cancel
	reconnectCancelsMu.Unlock()
	return id
}

func unregisterReconnectCancel(id uint64) {
	reconnectCancelsMu.Lock()
	delete(reconnectCancels, uintptr(id))
	reconnectCancelsMu.Unlock()
}

// ReconnectPolicy configures automatic link re-establishment.
type ReconnectPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
	OnFailed    func(*Link, error)
}

// WatchAndReconnect wraps closedCallback to re-Establish the link after closure.
func WatchAndReconnect(l *Link, tr TransportRef, policy ReconnectPolicy) {
	if l == nil || tr == nil {
		return
	}
	if policy.Backoff <= 0 {
		policy.Backoff = time.Second
	}
	orig := l.closedCallback
	l.SetLinkClosedCallback(func(link *Link) {
		if orig != nil {
			orig(link)
		}
		if GlobalPaused() {
			return
		}
		go reestablishLink(link, tr, policy)
	})
}

// TransportRef is the minimal transport surface needed for link reconnect.
type TransportRef interface {
	PrepareFreshPathRequest(destinationHash []byte) transport.PrepareFreshPathReturn
	ExpirePath(destinationHash []byte)
}

func reestablishLink(l *Link, tr TransportRef, policy ReconnectPolicy) {
	if l == nil || l.destination == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	id := registerReconnectCancel(cancel)
	defer func() {
		cancel()
		unregisterReconnectCancel(id)
	}()

	destHash := l.destination.GetHash()
	attempts := 0
	backoff := policy.Backoff
	for policy.MaxAttempts == 0 || attempts < policy.MaxAttempts {
		if ctx.Err() != nil {
			return
		}
		if GlobalPaused() {
			return
		}
		_ = tr.PrepareFreshPathRequest(destHash)
		if err := l.Reestablish(); err == nil {
			return
		} else if policy.OnFailed != nil {
			policy.OnFailed(l, err)
		}
		attempts++
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff *= 2
		if backoff > 5*time.Minute {
			backoff = 5 * time.Minute
		}
	}
}

// DestinationHash returns the destination hash for this link.
func (l *Link) DestinationHash() []byte {
	if l == nil || l.destination == nil {
		return nil
	}
	return l.destination.GetHash()
}
