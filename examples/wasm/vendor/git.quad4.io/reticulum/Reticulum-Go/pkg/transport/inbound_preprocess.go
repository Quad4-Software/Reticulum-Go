// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/health"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
)

func (t *Transport) classifyInboundTraffic(data []byte, iface common.NetworkInterface, packetType, destType byte) int {
	tc := TCData
	if packetType == PacketTypeAnnounce {
		tc = TCAnnounce
	} else if len(t.pathRequestDestHash) > 0 {
		if dh := destinationHashFromRaw(data); len(dh) > 0 && bytes.Equal(dh, t.pathRequestDestHash) {
			tc = TCPathRequest
		}
	}
	if iface != nil && t.ifaceStates != nil {
		if st := t.ifaceStates.get(iface.GetName()); st != nil && st.ingress != nil {
			if tc == TCAnnounce && st.ingress.InBurst() {
				tc = TCIngressLimited
			}
			if tc == TCPathRequest && ifaceIngressPRBurst(iface) {
				tc = TCIngressLimited
			}
		}
	}
	_ = destType
	return tc
}

func ifaceIngressPRBurst(iface common.NetworkInterface) bool {
	if pr, ok := iface.(interface{ PRBurstActive() bool }); ok {
		return pr.PRBurstActive()
	}
	return false
}

func destinationHashFromRaw(data []byte) []byte {
	if len(data) < 2 {
		return nil
	}
	headerByte := data[0]
	headerType := (headerByte & HeaderTypeMask) >> HeaderTypeShift
	ifacFlag := (headerByte & HeaderIFACMask) >> HeaderIFACShift
	start := HeaderSize
	if ifacFlag == 1 {
		start++
	}
	addrSize := AddrHashSize
	if headerType == 1 {
		addrSize = DoubleAddrSize
	}
	if len(data) < start+addrSize {
		return nil
	}
	return data[start : start+AddrHashSize]
}

func (t *Transport) preprocessInboundPacket(data []byte, iface common.NetworkInterface) (job packetJob, tc int, ok bool) {
	// Python counts inbound accounting against len(raw), the full wire
	// frame including any IFAC tag.
	wireLen := len(data)
	if len(data) < 2 {
		ifaceProtocolViolation(iface)
		t.filteredInbound(iface, wireLen)
		return packetJob{}, 0, false
	}
	if iface != nil && !iface.IsOnline() {
		t.filteredInbound(iface, wireLen)
		return packetJob{}, 0, false
	}

	unmasked, ifacOK := common.ApplyIFACInbound(iface, data)
	if !ifacOK {
		ifaceIFACViolation(iface)
		t.filteredInbound(iface, wireLen)
		return packetJob{}, 0, false
	}
	data = unmasked

	if iface != nil {
		if mtu := iface.GetMTU(); mtu > 0 && len(data) > mtu+64 {
			ifaceProtocolViolation(iface)
			t.filteredInbound(iface, wireLen)
			return packetJob{}, 0, false
		}
	}

	headerByte := data[0]
	packetType := headerByte & HeaderPacketTypeMask
	headerType := (headerByte & HeaderTypeMask) >> HeaderTypeShift
	destType := (headerByte & HeaderDestTypeMask) >> HeaderDestTypeShift

	// Copy into the pooled buffer before unpacking so the parsed packet's
	// fields alias memory that lives until the job is recycled.
	pc := getPacketCopy(len(data))
	copy(pc.buf, data)
	pkt := getInboundPacket()
	pkt.Raw = pc.buf
	if err := pkt.Unpack(); err != nil {
		ifaceProtocolViolation(iface)
		if iface != nil {
			health.Inc(iface.GetName(), health.KindUnpackFail)
		}
		t.filteredInbound(iface, wireLen)
		putPacketCopy(pc)
		putInboundPacket(pkt)
		return packetJob{}, 0, false
	}

	// Match Python Transport.packet_filter: PLAIN/GROUP payloads must not
	// travel more than one hop after inbound hop accounting. This must run
	// on the unmasked data above: on deferred-IFAC interfaces the raw frame
	// still carries the IFAC flag and bytes, so a pre-unmask check reads
	// IFAC bytes as the hops field.
	if packetType != PacketTypeAnnounce && (destType == DestTypePlain || destType == DestTypeGroup) {
		accounted := AccountInboundHops(data[1], iface)
		if accounted > 1 {
			if debug.Enabled(debug.DebugVerbose) {
				debug.Log(debug.DebugVerbose, "Dropped multi-hop PLAIN/GROUP packet",
					"dest_type", destType, "wire_hops", data[1], "accounted_hops", accounted)
			}
			ifaceProtocolViolation(iface)
			t.filteredInbound(iface, wireLen)
			putPacketCopy(pc)
			putInboundPacket(pkt)
			return packetJob{}, 0, false
		}
	}
	if !t.applyPacketFilter(pkt, iface) {
		ifacePacketFilterHit(iface)
		t.filteredInbound(iface, wireLen)
		putPacketCopy(pc)
		putInboundPacket(pkt)
		return packetJob{}, 0, false
	}

	// RNS 1.6.1: per-interface hop limiter before the global limiter, on
	// the raw wire hops field before inbound hop accounting.
	if iface != nil {
		if hl, ok := iface.(interface{ HopLimiter() (int, bool) }); ok {
			if lim, set := hl.HopLimiter(); set && lim > 0 && int(data[1]) >= lim {
				t.hopLimiterFiltered(iface, wireLen)
				putPacketCopy(pc)
				putInboundPacket(pkt)
				return packetJob{}, 0, false
			}
		}
	}
	if int(data[1]) >= t.effectiveHopLimiter() {
		t.hopLimiterFiltered(iface, wireLen)
		putPacketCopy(pc)
		putInboundPacket(pkt)
		return packetJob{}, 0, false
	}

	tc = t.classifyInboundTraffic(data, iface, packetType, destType)

	if packetType == PacketTypeAnnounce {
		if destType == DestTypePlain || destType == DestTypeGroup {
			ifaceProtocolViolation(iface)
			t.filteredInbound(iface, wireLen)
			putPacketCopy(pc)
			putInboundPacket(pkt)
			return packetJob{}, 0, false
		}
		// RNS 1.6.1 early announce checks, before queueing and expensive
		// validation. Hops here are still raw wire hops. The type-filter
		// local-client gate sees wire hops + 1 like Python post-increment.
		if len(data) > packet.MTU {
			ifaceProtocolViolation(iface)
			t.filteredInbound(iface, wireLen)
			putPacketCopy(pc)
			putInboundPacket(pkt)
			return packetJob{}, 0, false
		}
		if headerType == HeaderType1 && packet.MTU-len(data) < identity.TruncatedHashLength/8 {
			ifaceProtocolViolation(iface)
			t.filteredInbound(iface, wireLen)
			putPacketCopy(pc)
			putInboundPacket(pkt)
			return packetJob{}, 0, false
		}
		if len(pkt.Data) < identity.KeySize/8+identity.NameHashLength/8+10 {
			ifaceProtocolViolation(iface)
			t.filteredInbound(iface, wireLen)
			putPacketCopy(pc)
			putInboundPacket(pkt)
			return packetJob{}, 0, false
		}
		nameHash := pkt.Data[identity.KeySize/8 : identity.KeySize/8+identity.NameHashLength/8]
		if t.announcePrefilterDrop(pkt, nameHash, iface, int(pkt.Hops)+1, wireLen) {
			putPacketCopy(pc)
			putInboundPacket(pkt)
			return packetJob{}, 0, false
		}
	} else if tc == TCPathRequest {
		destHash, _, tag, _, prOK := parsePathRequestWire(pkt.Data)
		if !prOK {
			ifaceProtocolViolation(iface)
			t.filteredInbound(iface, wireLen)
			putPacketCopy(pc)
			putInboundPacket(pkt)
			return packetJob{}, 0, false
		}
		if len(tag) > identity.TruncatedHashLength/8 {
			ifaceProtocolViolation(iface)
		}
		pathRequestReceived(iface, wireLen)
		_ = destHash
	}

	// Python accepted_inbound counts len(raw) as rxb for every frame
	// preprocess accepts. Announce frames are the exception: Python
	// validates them inside preprocess (rejects to frxb), while Go
	// validates them in the worker, so their rxb/frxb is tallied there.
	if packetType != PacketTypeAnnounce {
		t.acceptedInbound(iface, wireLen)
	}
	return packetJob{
		pc:         pc,
		pkt:        pkt,
		iface:      iface,
		packetType: packetType,
		destType:   destType,
		headerType: headerType,
		wireLen:    wireLen,
	}, tc, true
}

func (t *Transport) submitInboundJob(job packetJob, tc int, block bool) bool {
	if t.inboundQueues != nil {
		if t.inboundQueues.put(tc, job) {
			return true
		}
		if block && t.inboundQueues.putWait(tc, job, t.done) {
			return true
		}
		putPacketCopy(job.pc)
		putInboundPacket(job.pkt)
		t.shedHandlerOverflow(job.iface)
		return false
	}
	if t.enqueuePacket(job, block) {
		return true
	}
	putPacketCopy(job.pc)
	putInboundPacket(job.pkt)
	t.shedHandlerOverflow(job.iface)
	return false
}
