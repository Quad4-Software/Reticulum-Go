// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
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
	entries map[hash16]*LinkRelayEntry
}

func newLinkRelayTable() *linkRelayTable {
	return &linkRelayTable{entries: make(map[hash16]*LinkRelayEntry)}
}

func (lt *linkRelayTable) put(linkID []byte, entry *LinkRelayEntry) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.entries[hash16FromSlice(linkID)] = entry
}

func (lt *linkRelayTable) get(linkID []byte) (*LinkRelayEntry, bool) {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	e, ok := lt.entries[hash16FromSlice(linkID)]
	return e, ok
}

func (lt *linkRelayTable) markValidated(linkID []byte) bool {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	e, ok := lt.entries[hash16FromSlice(linkID)]
	if !ok || e == nil {
		return false
	}
	e.Validated = true
	e.Timestamp = time.Now()
	return true
}

func (lt *linkRelayTable) touch(linkID []byte) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if e, ok := lt.entries[hash16FromSlice(linkID)]; ok && e != nil {
		e.Timestamp = time.Now()
	}
}

func (lt *linkRelayTable) delete(linkID []byte) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	delete(lt.entries, hash16FromSlice(linkID))
}

func (lt *linkRelayTable) sweep(maxIdle time.Duration) (expiredUnvalidated []*LinkRelayEntry, removed int) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	now := time.Now()
	for k, e := range lt.entries {
		if e == nil {
			delete(lt.entries, k)
			removed++
			continue
		}
		if !e.Validated {
			if now.After(e.ProofTimeout) {
				expiredUnvalidated = append(expiredUnvalidated, e)
				delete(lt.entries, k)
				removed++
			}
			continue
		}
		if now.Sub(e.Timestamp) > maxIdle {
			delete(lt.entries, k)
			removed++
		}
	}
	return expiredUnvalidated, removed
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
	if t.config != nil && t.config.ConnectedToSharedInstance {
		return false
	}
	if t.config == nil {
		return false
	}
	return t.config.EnableTransport
}

func (t *Transport) ourTransportID() []byte {
	if t.transportIdentity == nil {
		return nil
	}
	if len(t.transportIDCache) == 0 {
		t.transportIDCache = t.transportIdentity.Hash()
	}
	return t.transportIDCache
}

func rebuildHeaderType2(raw []byte, hops byte, nextHop []byte) ([]byte, error) {
	tail := identity.TruncatedHashLength/8 + 2
	if len(raw) < tail {
		return nil, errors.New("packet too short for HeaderType2 rewrite")
	}
	if len(nextHop) != identity.TruncatedHashLength/8 {
		return nil, fmt.Errorf("next hop must be %d bytes, got %d", identity.TruncatedHashLength/8, len(nextHop))
	}
	raw[1] = hops
	copy(raw[2:tail], nextHop)
	return raw, nil
}

// insertHeaderType2 upgrades a HeaderType1 wire packet to HeaderType2 by
// inserting the next-hop transport id after the hop byte. Matches Python
// Transport outbound wrapping when path hops > 1.
func insertHeaderType2(raw []byte, hops byte, nextHop []byte) ([]byte, error) {
	hopLen := identity.TruncatedHashLength / 8
	if len(raw) < 2 {
		return nil, errors.New("packet too short for HeaderType2 insert")
	}
	if len(nextHop) != hopLen {
		return nil, fmt.Errorf("next hop must be %d bytes, got %d", hopLen, len(nextHop))
	}
	newFlags := byte(0)
	newFlags |= (packet.HeaderType2 << 6) & packet.HeaderMaskHeaderType
	newFlags |= (packet.PropagationTransport << 4) & packet.HeaderMaskTransportType
	newFlags |= raw[0] & (packet.HeaderMaskContextFlag | 0x0F)
	out := make([]byte, 0, len(raw)+hopLen)
	out = append(out, newFlags, hops)
	out = append(out, nextHop...)
	out = append(out, raw[2:]...)
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
	copy(raw[2:], raw[tail:])
	raw[0] = newFlags
	raw[1] = hops
	return raw[:len(raw)-(identity.TruncatedHashLength/8)], nil
}

func rewriteHopsOnly(raw []byte, hops byte) []byte {
	if len(raw) < 2 {
		return raw
	}
	out := append([]byte(nil), raw...)
	out[1] = hops
	return out
}

func AccountInboundHops(wireHops byte, iface common.NetworkInterface) byte {
	if iface == nil {
		return wireHops
	}
	return linkRelayAccountedHops(wireHops, isLocalClientInterface(iface))
}

func linkRelayAccountedHops(wireHops byte, fromLocalClient bool) byte {
	if fromLocalClient {
		return wireHops
	}
	if wireHops < 0xFF {
		return wireHops + 1
	}
	return wireHops
}

// forwardTransportPacket relays HeaderType2 when TransportID matches
// ours. Returns true if handled (forwarded or dropped). False to fall
// through to local handling.
func (t *Transport) forwardTransportPacket(pkt *packet.Packet, raw []byte, sourceIface common.NetworkInterface) bool {
	if pkt == nil || pkt.HeaderType != packet.HeaderType2 || len(pkt.TransportID) == 0 {
		return false
	}
	ourID := t.ourTransportID()
	if ourID == nil {
		return false
	}
	if !bytes.Equal(pkt.TransportID, ourID) {
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Transport packet not for us, ignoring",
				"transport_id", fmt.Sprintf("%x", pkt.TransportID),
				"our_id", fmt.Sprintf("%x", ourID))
		}
		return false
	}
	if !t.transportEnabled() {
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Dropping transport packet: relay disabled",
				"dest_hash", fmt.Sprintf("%x", pkt.DestinationHash))
		}
		return true
	}

	destHash := pkt.DestinationHash
	if len(destHash) > identity.TruncatedHashLength/8 {
		destHash = destHash[:identity.TruncatedHashLength/8]
	}
	destKey := hash16FromSlice(destHash)

	t.mutex.RLock()
	path, hasPath := t.paths[pathMapKey(destHash)]
	_, isLocal := t.destinations[destKey]
	t.mutex.RUnlock()

	if isLocal {
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Transport packet absorbed (local destination)",
				"dest_hash", fmt.Sprintf("%x", destHash))
		}
		return false
	}
	if !hasPath || path == nil || path.Interface == nil {
		if debug.Enabled(debug.DebugInfo) {
			debug.Log(debug.DebugInfo, "No path for relayed transport packet, dropping",
				"dest_hash", fmt.Sprintf("%x", destHash))
		}
		return true
	}
	if path.Interface == sourceIface {
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Refusing to relay back onto receiving interface",
				"iface", sourceIface.GetName())
		}
		return true
	}

	fromLocal := isLocalClientInterface(sourceIface)
	newHops := linkRelayAccountedHops(pkt.Hops, fromLocal)
	if newHops >= MaxHops {
		if debug.Enabled(debug.DebugInfo) {
			debug.Log(debug.DebugInfo, "Transport packet exceeds MaxHops, dropping",
				"hops", newHops)
		}
		return true
	}

	rawCopy := append([]byte(nil), raw...)
	var out []byte
	var err error
	switch {
	case path.HopCount > 1:
		out, err = rebuildHeaderType2(rawCopy, newHops, path.NextHop)
	case path.HopCount == 1:
		out, err = stripHeaderType2(rawCopy, newHops)
	default:
		out = rewriteHopsOnly(rawCopy, newHops)
	}
	if err != nil {
		debug.Log(debug.DebugError, "Failed to rewrite transport packet",
			"error", err)
		return true
	}

	if pkt.PacketType == packet.PacketTypeLinkReq {
		out = clampRelayedLinkRequestMTU(out, pkt, sourceIface, path.Interface)
		t.recordLinkRelay(pkt, out, sourceIface, path, int(newHops))
	} else if pkt.PacketType != packet.PacketTypeAnnounce {
		t.recordReverseEntry(pkt, sourceIface, path.Interface)
	}

	if debug.Enabled(debug.DebugVerbose) {
		debug.Log(debug.DebugVerbose, "Relaying transport packet",
			"dest_hash", fmt.Sprintf("%x", destHash),
			"out_iface", path.Interface.GetName(),
			"hops_remaining", path.HopCount,
			"new_hops", newHops)
	}
	if sendErr := sendOnInterface(path.Interface, out, ""); sendErr != nil {
		debug.Log(debug.DebugError, "Failed to relay transport packet",
			"error", sendErr,
			"out_iface", path.Interface.GetName())
	}
	return true
}

func (t *Transport) recordLinkRelay(pkt *packet.Packet, raw []byte, recvIface common.NetworkInterface, path *common.Path, takenHops int) {
	if t.linkTable == nil {
		return
	}
	linkID := packet.LinkIDFromLinkRequest(pkt)
	if len(linkID) == 0 {
		return
	}
	now := time.Now()
	remaining := max(int(path.HopCount), 1)
	timeout := now.Add(LinkProofTimeoutPerHop * time.Duration(remaining))
	entry := &LinkRelayEntry{
		NextHop:         path.NextHop,
		NextHopIface:    path.Interface,
		ReceivedIface:   recvIface,
		RemainingHops:   remaining,
		TakenHops:       takenHops,
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
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Dropping link relay packet: transport disabled",
				"link_id", fmt.Sprintf("%x", linkID))
		}
		return true
	}

	fromLocal := isLocalClientInterface(sourceIface)
	accounted := linkRelayAccountedHops(raw[1], fromLocal)

	var outIface common.NetworkInterface
	switch {
	case entry.NextHopIface == entry.ReceivedIface:
		if int(accounted) != entry.RemainingHops && int(accounted) != entry.TakenHops {
			if debug.Enabled(debug.DebugVerbose) {
				debug.Log(debug.DebugVerbose, "Link relay hop mismatch on shared iface",
					"link_id", fmt.Sprintf("%x", linkID),
					"accounted_hops", accounted,
					"taken", entry.TakenHops,
					"remaining", entry.RemainingHops)
			}
			return true
		}
		outIface = entry.NextHopIface
	case sourceIface == entry.NextHopIface:
		if int(accounted) != entry.RemainingHops {
			if debug.Enabled(debug.DebugVerbose) {
				debug.Log(debug.DebugVerbose, "Link relay hop mismatch from next-hop iface",
					"link_id", fmt.Sprintf("%x", linkID),
					"accounted_hops", accounted,
					"remaining", entry.RemainingHops)
			}
			return true
		}
		outIface = entry.ReceivedIface
	case sourceIface == entry.ReceivedIface:
		if int(accounted) != entry.TakenHops {
			if debug.Enabled(debug.DebugVerbose) {
				debug.Log(debug.DebugVerbose, "Link relay hop mismatch from receive iface",
					"link_id", fmt.Sprintf("%x", linkID),
					"accounted_hops", accounted,
					"taken", entry.TakenHops)
			}
			return true
		}
		outIface = entry.NextHopIface
	default:
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Link relay: source iface unknown, dropping",
				"link_id", fmt.Sprintf("%x", linkID))
		}
		health.Inc("", health.KindLinkRelayUnknownIface)
		return true
	}
	if outIface == nil || !outIface.IsEnabled() {
		return true
	}

	out := rewriteHopsOnly(raw, accounted)
	debug.Log(debug.DebugInfo, "Relaying link data packet",
		"link_id", fmt.Sprintf("%x", linkID),
		"out_iface", outIface.GetName(),
		"hops", accounted)
	if err := sendOnInterface(outIface, out, ""); err != nil {
		debug.Log(debug.DebugError, "Failed to relay link data packet",
			"error", err,
			"out_iface", outIface.GetName())
	}
	t.linkTable.touch(linkID)
	return true
}

// relayBridgedLinkRequest forwards a HeaderType1 link request across another
// interface when this node has a known path but is not the destination.
func (t *Transport) relayBridgedLinkRequest(pkt *packet.Packet, raw []byte, sourceIface common.NetworkInterface) bool {
	if pkt == nil {
		return false
	}
	if pkt.HeaderType == packet.HeaderType2 {
		stripped, err := stripHeaderType2(append([]byte(nil), raw...), pkt.Hops)
		if err != nil {
			debug.Log(debug.DebugInfo, "Bridged link request HT2 strip failed", "error", err)
			return false
		}
		pkt = &packet.Packet{Raw: stripped}
		if err := pkt.Unpack(); err != nil {
			debug.Log(debug.DebugInfo, "Bridged link request unpack after strip failed", "error", err)
			ifaceName := ""
			if sourceIface != nil {
				ifaceName = sourceIface.GetName()
			}
			health.Inc(ifaceName, health.KindUnpackFail)
			return false
		}
		raw = stripped
	}
	return t.relayBridgedLinkRequestHT1(pkt, raw, sourceIface)
}

func (t *Transport) relayBridgedLinkRequestHT1(pkt *packet.Packet, raw []byte, sourceIface common.NetworkInterface) bool {
	if !t.transportEnabled() {
		return true
	}

	destHash := pkt.DestinationHash
	if len(destHash) > identity.TruncatedHashLength/8 {
		destHash = destHash[:identity.TruncatedHashLength/8]
	}
	destKey := hash16FromSlice(destHash)

	t.mutex.RLock()
	path, hasPath := t.paths[pathMapKey(destHash)]
	_, isLocal := t.destinations[destKey]
	t.mutex.RUnlock()
	if isLocal || !hasPath || path == nil || path.Interface == nil {
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Bridged link request not relayed",
				"dest_hash", fmt.Sprintf("%x", destHash),
				"is_local", isLocal,
				"has_path", hasPath,
				"path_iface_nil", path == nil || path.Interface == nil,
				"source_iface", sourceIface.GetName())
		}
		return false
	}
	if path.Interface == sourceIface {
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Bridged link request dropped: ingress equals egress",
				"dest_hash", fmt.Sprintf("%x", destHash),
				"iface", sourceIface.GetName())
		}
		return true
	}

	fromLocal := isLocalClientInterface(sourceIface)
	newHops := linkRelayAccountedHops(pkt.Hops, fromLocal)
	if newHops >= MaxHops {
		debug.Log(debug.DebugInfo, "Bridged link request exceeds MaxHops, dropping", "hops", newHops)
		return true
	}

	out := rewriteHopsOnly(raw, newHops)
	// Multi-hop paths need HeaderType2 with the next transport hop, matching
	// SendPacket and Python Transport outbound wrapping. Bare HT1 LRs are
	// dropped by mesh peers that only forward when transport_id matches.
	if path.HopCount > 1 && len(path.NextHop) > 0 && !bytes.Equal(path.NextHop, destHash) {
		wrapped, err := insertHeaderType2(raw, newHops, path.NextHop)
		if err != nil {
			debug.Log(debug.DebugError, "Failed to wrap bridged link request for transport",
				"error", err)
			return true
		}
		out = wrapped
	}
	t.recordLinkRelay(pkt, out, sourceIface, path, int(newHops))

	debug.Log(debug.DebugInfo, "Relaying bridged link request",
		"dest_hash", fmt.Sprintf("%x", destHash),
		"out_iface", path.Interface.GetName(),
		"hops", newHops,
		"path_hops", path.HopCount,
		"header_type", out[0]>>6,
		"from_local_client", fromLocal)
	if err := sendOnInterface(path.Interface, out, ""); err != nil {
		debug.Log(debug.DebugError, "Failed to relay bridged link request", "error", err)
	}
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
		if iface.ShouldEgressLimitPR() {
			if debug.Enabled(debug.DebugVerbose) {
				debug.Log(debug.DebugVerbose, "Skipping path-request rebroadcast due to egress limiting",
					"iface", iface.GetName(), "dest_hash", fmt.Sprintf("%x", destHash))
			}
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

func (t *Transport) queueDiscoveryPathRequest(destHash []byte, exclude common.NetworkInterface) {
	t.pendingDiscoveryPRMu.Lock()
	if len(t.pendingDiscoveryPRs) >= maxQueuedDiscoveryPRs {
		t.pendingDiscoveryPRMu.Unlock()
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Discovery PR queue full, dropping",
				"dest_hash", fmt.Sprintf("%x", destHash))
		}
		return
	}
	t.pendingDiscoveryPRs = append(t.pendingDiscoveryPRs, pendingDiscoveryPR{
		destHash: destHash,
		exclude:  exclude,
	})
	shouldStart := !t.discoveryDraining.Load()
	if shouldStart {
		t.discoveryDraining.Store(true)
	}
	t.pendingDiscoveryPRMu.Unlock()

	if shouldStart {
		go t.drainDiscoveryPRs()
	}
}

func (t *Transport) drainDiscoveryPRs() {
	defer t.discoveryDraining.Store(false)
	for {
		t.pendingDiscoveryPRMu.Lock()
		if len(t.pendingDiscoveryPRs) == 0 {
			t.pendingDiscoveryPRMu.Unlock()
			return
		}
		entry := t.pendingDiscoveryPRs[0]
		t.pendingDiscoveryPRs[0] = pendingDiscoveryPR{}
		t.pendingDiscoveryPRs = t.pendingDiscoveryPRs[1:]
		t.pendingDiscoveryPRMu.Unlock()

		t.rebroadcastPathRequest(entry.destHash, nil, nil, entry.exclude)
		time.Sleep(discoveryPRTxThrottle)
	}
}
