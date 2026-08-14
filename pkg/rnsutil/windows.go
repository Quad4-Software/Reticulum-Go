// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"context"
	"time"

	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/transport"
)

const (
	// LinkEstablishmentMargin is added to link.EstablishmentTimeout when an
	// application must wait for handshake completion.
	LinkEstablishmentMargin = 6 * time.Second
)

// FirstHopTimeout is the instance-aware first-hop wait. Shared-instance
// clients query the owner over RPC so the value reflects real interface
// bitrates instead of the local client socket.
func FirstHopTimeout(tr *transport.Transport, destHash []byte) time.Duration {
	sec := float64(transport.EstablishmentTimeoutPerHop)
	if tr != nil {
		sec = tr.FirstHopTimeout(destHash)
		if to, _, ok := sharedInstanceWindows(tr, destHash); ok {
			sec = to
		}
	}
	if sec < 0 {
		sec = float64(transport.EstablishmentTimeoutPerHop)
	}
	return time.Duration(sec * float64(time.Second))
}

// PathResponseWindow sizes a client wait for a path response. Shared-instance
// clients use the owner's first-hop timeout and slowest online bitrate.
func PathResponseWindow(tr *transport.Transport, destHash []byte) time.Duration {
	if tr == nil {
		return transport.PathResponseWindowFrom(float64(transport.EstablishmentTimeoutPerHop), 0)
	}
	firstHop := tr.FirstHopTimeout(destHash)
	bitrate := tr.SlowestOnlineBitrate()
	if to, br, ok := sharedInstanceWindows(tr, destHash); ok {
		firstHop = to
		if br > 0 {
			bitrate = br
		}
	}
	return transport.PathResponseWindowFrom(firstHop, bitrate)
}

// LinkEstablishmentWindow is link.EstablishmentTimeout plus a small margin.
func LinkEstablishmentWindow(l *link.Link) time.Duration {
	if l == nil {
		return LinkEstablishmentMargin
	}
	return l.EstablishmentTimeout() + LinkEstablishmentMargin
}

// BoundWait returns a child context limited to window unless parent already
// has a deadline (an explicit caller timeout wins). The returned cancel must
// be invoked by the caller when non-nil.
func BoundWait(parent context.Context, window time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if _, ok := parent.Deadline(); ok {
		return parent, func() {}
	}
	if window <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, window) // #nosec G118 -- caller defers cancel()
}

// CLIWaitContext applies timeout when positive. Zero means adaptive waits
// in WaitPathWindow and link establishment.
func CLIWaitContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return BoundWait(context.Background(), timeout)
}

// WaitPathWindow waits for a path using PathResponseWindow when ctx has no
// deadline.
func WaitPathWindow(ctx context.Context, tr *transport.Transport, destHash []byte) error {
	wait, cancel := BoundWait(ctx, PathResponseWindow(tr, destHash))
	defer cancel()
	return WaitPath(wait, tr, destHash)
}

func activateOutboundLink(ctx context.Context, l *link.Link) error {
	if err := l.Establish(); err != nil {
		return err
	}
	wait, cancel := BoundWait(ctx, LinkEstablishmentWindow(l))
	defer cancel()
	if err := WaitLinkActive(wait, l); err != nil {
		l.Teardown()
		return err
	}
	return nil
}

func sharedInstanceWindows(tr *transport.Transport, destHash []byte) (firstHop float64, bitrate int64, ok bool) {
	if tr == nil || !tr.ConnectedToSharedInstance() {
		return 0, 0, false
	}
	cfg := tr.GetConfig()
	if cfg == nil {
		return 0, 0, false
	}
	client, err := DialRPC(cfg, nil)
	if err != nil {
		return 0, 0, false
	}
	client.SetTimeout(2 * time.Second)
	to, err := client.GetFirstHopTimeout(destHash)
	if err != nil {
		return 0, 0, false
	}
	stats, err := client.GetInterfaceStats()
	if err == nil {
		bitrate = slowestOnlineBitrateFromStats(stats)
	}
	return to, bitrate, true
}

func slowestOnlineBitrateFromStats(stats transport.InterfaceStatsResponse) int64 {
	var slowest int64
	found := false
	for _, st := range stats.Interfaces {
		if !st.Status || st.Bitrate <= 0 {
			continue
		}
		if !found || st.Bitrate < slowest {
			slowest = st.Bitrate
			found = true
		}
	}
	if !found {
		return 0
	}
	return slowest
}
