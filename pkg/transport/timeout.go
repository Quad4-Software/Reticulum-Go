// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
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
// enabled interface, or 0 if none advertise a positive rate.
func (t *Transport) SlowestOnlineBitrate() int64 {
	if t == nil {
		return 0
	}
	var slowest int64
	found := false
	for _, e := range t.snapshotRegisteredInterfaces() {
		if e.iface == nil || !e.iface.IsEnabled() || !e.iface.IsOnline() {
			continue
		}
		br := interfaceBitrate(e.iface)
		if br <= 0 {
			continue
		}
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

// PathResponseWindow sizes a client wait for a cold or first path response.
// It is the max of FirstHopTimeout, a two-way airtime estimate on the slowest
// online interface (clamped to Reticulum's 5 bit/s minimum), and
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
