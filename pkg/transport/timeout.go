// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"context"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
)

// FirstHopTimeout returns the first-hop wait in seconds for destinationHash.
// Matches Python Transport.first_hop_timeout: packet MTU airtime on the next
// hop interface plus DEFAULT_PER_HOP_TIMEOUT. With no next-hop bitrate this
// is EstablishmentTimeoutPerHop (6s). Shared-instance clients should call
// this on the instance that owns the real interfaces (RPC get first_hop_timeout)
// rather than on the local client socket.
func (t *Transport) FirstHopTimeout(destinationHash []byte) float64 {
	fallback := float64(EstablishmentTimeoutPerHop)
	if t == nil {
		return fallback
	}
	br := interfaceBitrate(t.nextHopIface(destinationHash))
	if br <= 0 {
		return fallback
	}
	return float64(packet.MTU)*8/float64(br) + fallback
}

// SlowestOnlineBitrate returns the lowest advertised bitrate of an online
// enabled outgoing interface, or 0 if none advertise a positive rate.
// Receive-only interfaces are skipped because they cannot carry the request.
func (t *Transport) SlowestOnlineBitrate() int64 {
	if t == nil {
		return 0
	}
	var slowest int64
	found := false
	for _, e := range t.snapshotRegisteredInterfaces() {
		if !ifaceCountsForTimeout(e.iface) {
			continue
		}
		br := interfaceBitrate(e.iface)
		if !found || br < slowest {
			slowest = br
			found = true
		}
	}
	if !found {
		return 0
	}
	return slowest
}

// DiscoveryTimeout is how long a recursive unknown-path search is kept.
// Floor is PathRequestTimeout. Each interface the request will actually
// leave on can raise that to a two-way MTU airtime plus PathRequestGrace
// so a slow last hop is not abandoned at 15 seconds.
func (t *Transport) DiscoveryTimeout(attached common.NetworkInterface) time.Duration {
	timeout := time.Duration(PathRequestTimeout) * time.Second
	if t == nil {
		return timeout
	}
	for _, iface := range t.discoveryFanoutIfaces(attached) {
		br := interfaceBitrate(iface)
		if br <= 0 {
			continue
		}
		if medium := mediumRoundTripTimeout(br); medium > timeout {
			timeout = medium
		}
	}
	return timeout
}

// ExtraLinkProofTimeout is one-way MTU airtime on iface, matching Python
// Transport.extra_link_proof_timeout. Use the outbound hop, not the
// interface the link request arrived on.
func ExtraLinkProofTimeout(iface common.NetworkInterface) time.Duration {
	br := interfaceBitrate(iface)
	if br <= 0 {
		return 0
	}
	if minBr := int64(common.BitrateMinimum); br < minBr {
		br = minBr
	}
	return time.Duration(float64(packet.MTU) * 8 / float64(br) * float64(time.Second))
}

// AwaitPath requests a path if needed and blocks until HasPath is true or
// ctx is done. When ctx has no deadline the wait is PathResponseWindow so
// callers do not invent a flat 15 second timer.
func (t *Transport) AwaitPath(ctx context.Context, destinationHash []byte) error {
	if t == nil {
		return common.ErrNoPathToDestination
	}
	if t.HasPath(destinationHash) {
		return nil
	}
	wait, cancel := waitContext(ctx, t.PathResponseWindow(destinationHash))
	defer cancel()
	const requestEvery = 2 * time.Second
	_ = t.RequestPath(destinationHash, "", nil, false)
	lastReq := time.Now()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if t.HasPath(destinationHash) {
			return nil
		}
		select {
		case <-wait.Done():
			return wait.Err()
		case <-ticker.C:
			if time.Since(lastReq) >= requestEvery {
				_ = t.RequestPath(destinationHash, "", nil, false)
				lastReq = time.Now()
			}
		}
	}
}

// PathResponseWindow sizes a client wait for a cold or first path response.
// It is the max of FirstHopTimeout, a two-way airtime estimate on the slowest
// online outgoing interface (clamped to Reticulum's 5 bit/s minimum), and
// PathRequestTimeout. Mixed fast and slow interfaces wait for the slowest
// because the next hop is unknown until the response arrives.
func (t *Transport) PathResponseWindow(destinationHash []byte) time.Duration {
	if t == nil {
		return PathResponseWindowFrom(float64(EstablishmentTimeoutPerHop), 0)
	}
	return PathResponseWindowFrom(t.FirstHopTimeout(destinationHash), t.SlowestOnlineBitrate())
}

// PathResponseWindowFrom computes PathResponseWindow from already-resolved
// first-hop seconds and a slowest online bitrate. bitrate 0 skips the
// airtime term.
func PathResponseWindowFrom(firstHopSec float64, bitrate int64) time.Duration {
	window := firstHopSec
	if bitrate > 0 {
		br := float64(bitrate)
		if minBr := float64(common.BitrateMinimum); br < minBr {
			br = minBr
		}
		air := 2*(float64(PathExchangeBytes)*8/br) + float64(PathWindowMarginSec)
		if air > window {
			window = air
		}
	}
	if window < float64(PathRequestTimeout) {
		window = float64(PathRequestTimeout)
	}
	if window < 0 {
		window = float64(PathRequestTimeout)
	}
	return time.Duration(window * float64(time.Second))
}

func (t *Transport) nextHopIface(destinationHash []byte) common.NetworkInterface {
	if t == nil || len(destinationHash) == 0 {
		return nil
	}
	t.mutex.RLock()
	path, ok := t.livePath(destinationHash, time.Now())
	var iface common.NetworkInterface
	if ok && path != nil {
		iface = path.Interface
	}
	t.mutex.RUnlock()
	if iface != nil {
		return iface
	}
	name := t.NextHopInterface(destinationHash)
	if name == "" {
		return nil
	}
	got, err := t.GetInterface(name)
	if err != nil {
		return nil
	}
	return got
}

func interfaceBitrate(iface common.NetworkInterface) int64 {
	if iface == nil {
		return 0
	}
	switch br := iface.(type) {
	case interface{ GetBitrate() int }:
		return int64(br.GetBitrate())
	case interface{ GetBitrate() int64 }:
		return br.GetBitrate()
	case interface{ GetBitrate() uint64 }:
		return int64(br.GetBitrate()) // #nosec G115 -- timeout sizing only
	}
	return 0
}

func ifaceCountsForTimeout(iface common.NetworkInterface) bool {
	if iface == nil || !iface.IsEnabled() || !iface.IsOnline() {
		return false
	}
	if !common.InterfaceAllowsOutgoing(iface) {
		return false
	}
	return interfaceBitrate(iface) > 0
}

func mediumRoundTripTimeout(bitrate int64) time.Duration {
	br := float64(bitrate)
	if minBr := float64(common.BitrateMinimum); br < minBr {
		br = minBr
	}
	sec := 2*(float64(packet.MTU)*8/br) + PathRequestGrace.Seconds()
	return time.Duration(sec * float64(time.Second))
}

func waitContext(parent context.Context, window time.Duration) (context.Context, context.CancelFunc) {
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
