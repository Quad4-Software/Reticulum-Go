// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"sync/atomic"
	"time"

	"quad4/reticulum-go/pkg/transport"
)

var globalPaused atomic.Bool

// SetGlobalPaused freezes link watchdog stale timers during known sleep.
func SetGlobalPaused(paused bool) {
	globalPaused.Store(paused)
}

// GlobalPaused reports whether link watchdog timers are frozen.
func GlobalPaused() bool {
	return globalPaused.Load()
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
	destHash := l.destination.GetHash()
	attempts := 0
	backoff := policy.Backoff
	for policy.MaxAttempts == 0 || attempts < policy.MaxAttempts {
		_ = tr.PrepareFreshPathRequest(destHash)
		if err := l.Reestablish(); err == nil {
			return
		} else if policy.OnFailed != nil {
			policy.OnFailed(l, err)
		}
		attempts++
		time.Sleep(backoff)
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
