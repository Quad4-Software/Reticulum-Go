// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"crypto/rand"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/packet"
)

// initLocalHopsDelta picks a random hop delta in [2, 7] when enabled.
func (t *Transport) initLocalHopsDelta() {
	if t == nil || t.config == nil || !t.config.LocalHopsDelta {
		return
	}
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.localHopsDelta = 2
		return
	}
	t.localHopsDelta = int((b[0] % 6) + 2)
	debug.Log(debug.DebugVerbose, "Local hops delta enabled", "delta", t.localHopsDelta)
}

// shouldApplyLocalHopsDelta applies to locally originated hop-0 packets leaving
// a non-shared-instance owner.
func (t *Transport) shouldApplyLocalHopsDelta(p *packet.Packet, iface common.NetworkInterface) bool {
	if t == nil || t.localHopsDelta == 0 || p == nil {
		return false
	}
	if t.config != nil && t.config.ConnectedToSharedInstance {
		return false
	}
	if p.Hops != 0 {
		return false
	}
	if p.DestinationType == packet.DestinationPlain || p.DestinationType == packet.DestinationGroup {
		return false
	}
	if iface != nil && isLocalClientInterface(iface) {
		return false
	}
	return true
}

func isLocalClientInterface(iface common.NetworkInterface) bool {
	if iface == nil {
		return false
	}
	switch iface.GetType() {
	case common.IFTypeUnix:
		return true
	default:
		return false
	}
}

// applyLocalHopsDeltaIfNeeded sets p.Hops to the configured delta when applicable.
func (t *Transport) applyLocalHopsDeltaIfNeeded(p *packet.Packet, iface common.NetworkInterface) {
	if !t.shouldApplyLocalHopsDelta(p, iface) {
		return
	}
	delta := min(max(t.localHopsDelta, 2), 7)
	p.Hops = byte(delta) // #nosec G115 -- clamped to 2-7
	p.Packed = false
}
