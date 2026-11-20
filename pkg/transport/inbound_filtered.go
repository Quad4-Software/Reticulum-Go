// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
)

// spawnedParentOf returns the parent interface a spawned interface
// propagates counters to (Python interface.parent_interface), or nil.
func spawnedParentOf(iface common.NetworkInterface) common.NetworkInterface {
	if p, ok := iface.(interface {
		SpawnedParent() common.NetworkInterface
	}); ok {
		return p.SpawnedParent()
	}
	return nil
}

// acceptedInbound counts an accepted inbound frame's wire bytes on the
// receiving interface and its spawned parent (Python accepted_inbound /
// interface.rxb). Filtered frames count frxb instead, never rxb.
func (t *Transport) acceptedInbound(iface common.NetworkInterface, size int) {
	if size < 0 {
		size = 0
	}
	for i := iface; i != nil; i = spawnedParentOf(i) {
		if ta, ok := i.(interface{ AcceptedRxTally(int) }); ok {
			ta.AcceptedRxTally(size)
		}
	}
}

// filteredInbound counts a dropped inbound frame's bytes on the receiving
// interface, its spawned parent and the transport-wide filtered totals
// (Python filtered_inbound / Transport.filtered_rxb).
func (t *Transport) filteredInbound(iface common.NetworkInterface, size int) {
	if size < 0 {
		size = 0
	}
	for i := iface; i != nil; i = spawnedParentOf(i) {
		if fa, ok := i.(interface{ FilteredRxTally(int) }); ok {
			fa.FilteredRxTally(size)
		}
	}
	t.filteredRxB.Add(uint64(size))
}

// filteredInboundAnnounce counts a filtered announce (Python
// Interface.filtered_announce / Transport.filtered_arxc).
func (t *Transport) filteredInboundAnnounce(iface common.NetworkInterface) {
	for i := iface; i != nil; i = spawnedParentOf(i) {
		if fa, ok := i.(interface{ FilteredAnnounce() }); ok {
			fa.FilteredAnnounce()
		}
	}
	t.filteredARXC.Add(1)
}

// announceReceived mirrors Python Interface.received_announce: arxc/arxb on
// the receiving interface and every spawned parent.
func announceReceived(iface common.NetworkInterface, size int) {
	for i := iface; i != nil; i = spawnedParentOf(i) {
		if ra, ok := i.(interface{ ReceivedAnnounceBytes(int) }); ok {
			ra.ReceivedAnnounceBytes(size)
		} else if ra, ok := i.(interface{ ReceivedAnnounce() }); ok {
			ra.ReceivedAnnounce()
		}
	}
}

// announceSent mirrors Python Interface.sent_announce propagation.
func announceSent(iface common.NetworkInterface, size int) {
	for i := iface; i != nil; i = spawnedParentOf(i) {
		if sa, ok := i.(interface{ SentAnnounceBytes(int) }); ok {
			sa.SentAnnounceBytes(size)
		} else if sa, ok := i.(interface{ SentAnnounce() }); ok {
			sa.SentAnnounce()
		}
	}
}

// pathRequestReceived mirrors Python Interface.received_path_request
// propagation.
func pathRequestReceived(iface common.NetworkInterface, size int) {
	for i := iface; i != nil; i = spawnedParentOf(i) {
		if pr, ok := i.(interface{ ReceivedPathRequestBytes(int) }); ok {
			pr.ReceivedPathRequestBytes(size)
		}
	}
}

// pathRequestSent mirrors Python Interface.sent_path_request propagation.
func pathRequestSent(iface common.NetworkInterface, size int) {
	for i := iface; i != nil; i = spawnedParentOf(i) {
		if sp, ok := i.(interface{ SentPathRequestBytes(int) }); ok {
			sp.SentPathRequestBytes(size)
		} else {
			i.SentPathRequest()
		}
	}
}

func (t *Transport) hopLimiterFiltered(iface common.NetworkInterface, size int) {
	if iface != nil {
		if hl, ok := iface.(interface{ HopLimiterHit() }); ok {
			hl.HopLimiterHit()
		}
	}
	t.filteredInbound(iface, size)
}

func ifacePacketFilterHit(iface common.NetworkInterface) {
	if iface != nil {
		if pf, ok := iface.(interface{ PacketFilterHit() }); ok {
			pf.PacketFilterHit()
		}
	}
}

// announcePrefilterDrop runs the RNS 1.6.1 cheap announce gates that decide
// a packet never reaches the announce worker: blocked type, required_only
// type gate, and the known-destination announce_prefilter. hops is the
// post-increment hop count used by the local-client exception. wireLen is
// the raw wire frame length used for filtered byte accounting.
func (t *Transport) announcePrefilterDrop(pkt *packet.Packet, nameHash []byte, iface common.NetworkInterface, hops int, wireLen int) bool {
	if t.destFilters != nil {
		if t.destFilters.isBlockedNameHash(nameHash) {
			t.filteredInbound(iface, wireLen)
			return true
		}
		if t.filterRequiredOnly &&
			!t.destFilters.announceAllowed(nameHash, hops, true, iface != nil && isLocalClientInterface(iface)) {
			t.filteredInboundAnnounce(iface)
			t.filteredInbound(iface, wireLen)
			return true
		}
	}

	destKey := hash16FromSlice(pkt.DestinationHash)
	t.mutex.RLock()
	path, pathKnown := t.paths[pathMapKey(pkt.DestinationHash)]
	_, inPathReqs := t.lastPathRequest[pathMapKey(pkt.DestinationHash)]
	_, inDiscoveryReqs := t.discoveryPathRequests[destKey]
	_, inAnnounceTable := t.announceTable[destKey]
	pathState := t.pathStates[pathMapKey(pkt.DestinationHash)]
	t.mutex.RUnlock()
	if !inAnnounceTable {
		// Python announce_table membership also covers announces awaiting
		// retransmission. Go queues those as delayed forward jobs.
		inAnnounceTable = t.hasPendingAnnounceForward(destKey)
	}

	if !pathKnown || path == nil {
		return false
	}
	// prefilter_conditions from Transport.preprocess_inbound: path responses
	// and announces for destinations with outstanding requests always pass.
	if pkt.Context == packet.ContextPathResponse || inPathReqs || inDiscoveryReqs || inAnnounceTable {
		return false
	}
	if t.announcePrefilterPasses(pkt, path, iface, pathState) {
		return false
	}
	// Keep the losing copy as a failover alternate. It re-enters full
	// validation if the current path goes unresponsive.
	curIfaceName := ""
	if path.Interface != nil {
		curIfaceName = path.Interface.GetName()
	}
	t.notePendingAltAnnounce(pkt.DestinationHash, pkt.Raw, iface, pkt.Hops, curIfaceName)
	// Dropped before doing useful work: count the announce as received with
	// size 0 (arxc still increments), then as a filtered announce.
	announceReceived(iface, 0)
	t.filteredInboundAnnounce(iface)
	t.filteredInbound(iface, wireLen)
	return true
}

// heldAnnounceUseless reports whether a held announce for dest can no longer
// win against the known path. Python 1.6.1 culls those from the held queue so
// useful new announces keep their slots. Unknown paths keep their entries.
func (t *Transport) heldAnnounceUseless(dest hash16, data []byte, recvIface common.NetworkInterface) bool {
	pkt := &packet.Packet{Raw: data}
	if err := pkt.Unpack(); err != nil {
		return false
	}
	t.mutex.RLock()
	path, pathKnown := t.paths[pathMapKey(dest.bytes[:])]
	pathState := t.pathStates[pathMapKey(dest.bytes[:])]
	t.mutex.RUnlock()
	if !pathKnown || path == nil {
		return false
	}
	return !t.announcePrefilterPasses(pkt, path, recvIface, pathState)
}

// announcePrefilterPasses mirrors Python Transport.announce_prefilter:
// newer announces pass, older do not, equal timebases prefer higher-gravity
// receiving interfaces, and unresponsive paths accept equal announces.
func (t *Transport) announcePrefilterPasses(pkt *packet.Packet, path *common.Path, recvIface common.NetworkInterface, pathState byte) bool {
	if path.Interface == nil {
		return true
	}
	var randomBlob []byte
	if len(pkt.Data) >= identity.KeySize/8+identity.NameHashLength/8+10 {
		randomBlob = pkt.Data[identity.KeySize/8+identity.NameHashLength/8 : identity.KeySize/8+identity.NameHashLength/8+10]
	}
	emitted := announceEmitted(randomBlob)
	pathTimebase := timebaseFromRandomBlobs(path.RandomBlobs)
	if emitted > pathTimebase {
		return true
	}
	if emitted < pathTimebase {
		return false
	}
	newGravity, oldGravity := 0, 0
	if recvIface != nil {
		if g, ok := recvIface.(interface{ GetGravity() int }); ok {
			newGravity = g.GetGravity()
		}
	}
	if g, ok := path.Interface.(interface{ GetGravity() int }); ok {
		oldGravity = g.GetGravity()
	}
	if newGravity > oldGravity {
		return true
	}
	return pathState == StateUnresponsive
}
