// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
)

// LinkRelayEntry is one row in the transit link relay table (link_table).
type LinkRelayEntry struct {
	NextHop         []byte
	NextHopIface    common.NetworkInterface
	ReceivedIface   common.NetworkInterface
	RemainingHops   int
	TakenHops       int
	DestinationHash []byte
	Validated       bool
	ProofTimeout    time.Time
	Timestamp       time.Time
	OriginalLinkID  []byte
}

type linkRelayTable struct {
	mu      sync.RWMutex
	entries map[string]*LinkRelayEntry
}

func newLinkRelayTable() *linkRelayTable {
	return &linkRelayTable{entries: make(map[string]*LinkRelayEntry)}
}

func (lt *linkRelayTable) put(linkID []byte, entry *LinkRelayEntry) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.entries[string(linkID)] = entry
}

func (lt *linkRelayTable) get(linkID []byte) (*LinkRelayEntry, bool) {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	e, ok := lt.entries[string(linkID)]
	return e, ok
}

func (lt *linkRelayTable) delete(linkID []byte) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	delete(lt.entries, string(linkID))
}

func (lt *linkRelayTable) sweep(maxIdle time.Duration) int {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	now := time.Now()
	removed := 0
	for k, e := range lt.entries {
		if now.After(e.ProofTimeout) && now.Sub(e.Timestamp) > maxIdle {
			delete(lt.entries, k)
			removed++
		}
	}
	return removed
}

func (lt *linkRelayTable) removeEntriesReferencing(iface common.NetworkInterface) {
	if lt == nil || iface == nil {
		return
	}
	lt.mu.Lock()
	defer lt.mu.Unlock()
	for k, e := range lt.entries {
		if e == nil {
			continue
		}
		if e.NextHopIface == iface || e.ReceivedIface == iface {
			delete(lt.entries, k)
		}
	}
}

func (t *Transport) transportEnabled() bool {
	if t.config == nil {
		return false
	}
	return t.config.EnableTransport
}

func (t *Transport) ourTransportID() []byte {
	if t.transportIdentity == nil {
		return nil
	}
	return t.transportIdentity.Hash()
}

func rebuildHeaderType2(raw []byte, hops byte, nextHop []byte) ([]byte, error) {
	tail := identity.TruncatedHashLength/8 + 2
	if len(raw) < tail {
		return nil, errors.New("packet too short for HeaderType2 rewrite")
	}
	if len(nextHop) != identity.TruncatedHashLength/8 {
		return nil, fmt.Errorf("next hop must be %d bytes, got %d", identity.TruncatedHashLength/8, len(nextHop))
	}
	out := make([]byte, 0, len(raw))
	out = append(out, raw[0])
	out = append(out, hops)
	out = append(out, nextHop...)
	out = append(out, raw[tail:]...)
	return out, nil
}

func stripHeaderType2(raw []byte, hops byte) ([]byte, error) {
	tail := identity.TruncatedHashLength/8 + 2
	if len(raw) < tail {
		return nil, errors.New("packet too short for HeaderType2 strip")
	}
	newFlags := byte(0)
	newFlags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	newFlags |= (packet.PropagationBroadcast << 4) & packet.HeaderMaskTransportType
	newFlags |= raw[0] & 0x0F
	out := make([]byte, 0, len(raw)-(identity.TruncatedHashLength/8))
	out = append(out, newFlags, hops)
	out = append(out, raw[tail:]...)
	return out, nil
}

func rewriteHopsOnly(raw []byte, hops byte) []byte {
	if len(raw) < 2 {
		return raw
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	out[1] = hops
	return out
}

// forwardTransportPacket relays HeaderType2 when TransportID matches
// ours. Returns true if handled (forwarded or dropped); false to fall
// through to local handling.
func (t *Transport) forwardTransportPacket(pkt *packet.Packet, raw []byte, sourceIface common.NetworkInterface) bool {
	if pkt == nil || pkt.HeaderType != packet.HeaderType2 || len(pkt.TransportID) == 0 {
		return false
	}
	ourID := t.ourTransportID()
	if ourID == nil {
		return false
	}
	if string(pkt.TransportID) != string(ourID) {
		debug.Log(debug.DebugVerbose, "Transport packet not for us, ignoring",
			"transport_id", fmt.Sprintf("%x", pkt.TransportID),
			"our_id", fmt.Sprintf("%x", ourID))
		return false
	}
	if !t.transportEnabled() {
		debug.Log(debug.DebugVerbose, "Dropping transport packet: relay disabled",
			"dest_hash", fmt.Sprintf("%x", pkt.DestinationHash))
		return true
	}

	destHash := pkt.DestinationHash
	if len(destHash) > identity.TruncatedHashLength/8 {
		destHash = destHash[:identity.TruncatedHashLength/8]
	}

	t.mutex.RLock()
	path, hasPath := t.paths[string(destHash)]
	_, isLocal := t.destinations[string(destHash)]
	t.mutex.RUnlock()

	if isLocal {
		debug.Log(debug.DebugVerbose, "Transport packet absorbed (local destination)",
			"dest_hash", fmt.Sprintf("%x", destHash))
		return false
	}
	if !hasPath || path == nil || path.Interface == nil {
		debug.Log(debug.DebugInfo, "No path for relayed transport packet, dropping",
			"dest_hash", fmt.Sprintf("%x", destHash))
		return true
	}
	if path.Interface == sourceIface {
		debug.Log(debug.DebugVerbose, "Refusing to relay back onto receiving interface",
			"iface", sourceIface.GetName())
		return true
	}

	newHops := pkt.Hops + 1
	if newHops >= MaxHops {
		debug.Log(debug.DebugInfo, "Transport packet exceeds MaxHops, dropping",
			"hops", newHops)
		return true
	}

	var out []byte
	var err error
	switch {
	case path.HopCount > 1:
		out, err = rebuildHeaderType2(raw, newHops, path.NextHop)
	case path.HopCount == 1:
		out, err = stripHeaderType2(raw, newHops)
	default:
		out = rewriteHopsOnly(raw, newHops)
	}
	if err != nil {
		debug.Log(debug.DebugError, "Failed to rewrite transport packet",
			"error", err)
		return true
	}

	if pkt.PacketType == packet.PacketTypeLinkReq {
		t.recordLinkRelay(pkt, raw, sourceIface, path)
	}

	debug.Log(debug.DebugInfo, "Relaying transport packet",
		"dest_hash", fmt.Sprintf("%x", destHash),
		"out_iface", path.Interface.GetName(),
		"hops_remaining", path.HopCount,
		"new_hops", newHops)
	if sendErr := path.Interface.Send(out, ""); sendErr != nil {
		debug.Log(debug.DebugError, "Failed to relay transport packet",
			"error", sendErr,
			"out_iface", path.Interface.GetName())
	}
	return true
}

func (t *Transport) recordLinkRelay(pkt *packet.Packet, raw []byte, recvIface common.NetworkInterface, path *common.Path) {
	if t.linkTable == nil {
		return
	}
	linkID := linkIDFromLinkRequest(pkt, raw)
	if len(linkID) == 0 {
		return
	}
	now := time.Now()
	remaining := int(path.HopCount)
	if remaining < 1 {
		remaining = 1
	}
	timeout := now.Add(LinkProofTimeoutPerHop * time.Duration(remaining))
	entry := &LinkRelayEntry{
		NextHop:         path.NextHop,
		NextHopIface:    path.Interface,
		ReceivedIface:   recvIface,
		RemainingHops:   remaining,
		TakenHops:       int(pkt.Hops),
		DestinationHash: append([]byte(nil), pkt.DestinationHash...),
		Validated:       false,
		ProofTimeout:    timeout,
		Timestamp:       now,
		OriginalLinkID:  append([]byte(nil), linkID...),
	}
	t.linkTable.put(linkID, entry)
	debug.Log(debug.DebugInfo, "Registered relayed link",
		"link_id", fmt.Sprintf("%x", linkID),
		"remaining_hops", remaining,
		"recv_iface", recvIface.GetName(),
		"next_hop_iface", path.Interface.GetName())
}

func linkIDFromLinkRequest(pkt *packet.Packet, raw []byte) []byte {
	if pkt == nil {
		return nil
	}
	dest := pkt.DestinationHash
	if len(dest) == 0 || len(pkt.Data) == 0 {
		return nil
	}
	hasher := sha256.New()
	hasher.Write(dest)
	hasher.Write(pkt.Data)
	h := hasher.Sum(nil)
	if len(h) >= identity.TruncatedHashLength/8 {
		return h[:identity.TruncatedHashLength/8]
	}
	return h
}

func (t *Transport) forwardLinkData(raw []byte, sourceIface common.NetworkInterface) bool {
	if t.linkTable == nil || len(raw) < identity.TruncatedHashLength/8+2 {
		return false
	}
	linkID := raw[2 : identity.TruncatedHashLength/8+2]
	entry, ok := t.linkTable.get(linkID)
	if !ok {
		return false
	}
	if !t.transportEnabled() {
		debug.Log(debug.DebugVerbose, "Dropping link relay packet: transport disabled",
			"link_id", fmt.Sprintf("%x", linkID))
		return true
	}

	var outIface common.NetworkInterface
	switch {
	case entry.NextHopIface == entry.ReceivedIface:
		outIface = entry.NextHopIface
	case sourceIface == entry.NextHopIface:
		outIface = entry.ReceivedIface
	case sourceIface == entry.ReceivedIface:
		outIface = entry.NextHopIface
	default:
		debug.Log(debug.DebugVerbose, "Link relay: source iface unknown, dropping",
			"link_id", fmt.Sprintf("%x", linkID))
		return true
	}
	if outIface == nil || !outIface.IsEnabled() {
		return true
	}

	newRaw := make([]byte, len(raw))
	copy(newRaw, raw)
	if newRaw[1] < 0xFF {
		newRaw[1]++
	}

	debug.Log(debug.DebugInfo, "Relaying link data packet",
		"link_id", fmt.Sprintf("%x", linkID),
		"out_iface", outIface.GetName())
	if err := outIface.Send(newRaw, ""); err != nil {
		debug.Log(debug.DebugError, "Failed to relay link data packet",
			"error", err,
			"out_iface", outIface.GetName())
	}
	entry.Timestamp = time.Now()
	return true
}

func (t *Transport) rebroadcastPathRequest(destHash, requestorTransportID, tag []byte, exclude common.NetworkInterface) {
	if !t.transportEnabled() {
		return
	}
	t.mutex.RLock()
	ifaces := make([]common.NetworkInterface, 0, len(t.interfaces))
	for _, iface := range t.interfaces {
		if iface == exclude || !iface.IsEnabled() {
			continue
		}
		ifaces = append(ifaces, iface)
	}
	t.mutex.RUnlock()
	if len(ifaces) == 0 {
		return
	}
	for _, iface := range ifaces {
		if err := t.RequestPath(destHash, iface.GetName(), tag, true); err != nil {
			debug.Log(debug.DebugVerbose, "Path-request rebroadcast failed",
				"iface", iface.GetName(), "error", err)
		}
	}
}
