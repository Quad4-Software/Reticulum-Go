// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
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
	if len(data) < 2 {
		ifaceProtocolViolation(iface)
		return packetJob{}, 0, false
	}
	if iface != nil && !iface.IsOnline() {
		return packetJob{}, 0, false
	}

	unmasked, ifacOK := common.ApplyIFACInbound(iface, data)
	if !ifacOK {
		ifaceIFACViolation(iface)
		return packetJob{}, 0, false
	}
	data = unmasked

	if iface != nil {
		if mtu := iface.GetMTU(); mtu > 0 && len(data) > mtu+64 {
			ifaceProtocolViolation(iface)
			return packetJob{}, 0, false
		}
	}

	headerByte := data[0]
	packetType := headerByte & HeaderPacketTypeMask
	headerType := (headerByte & HeaderTypeMask) >> HeaderTypeShift
	destType := (headerByte & HeaderDestTypeMask) >> HeaderDestTypeShift

	pkt := &packet.Packet{Raw: data}
	if err := pkt.Unpack(); err != nil {
		ifaceProtocolViolation(iface)
		if iface != nil {
			health.Inc(iface.GetName(), health.KindUnpackFail)
		}
		return packetJob{}, 0, false
	}
	if !t.applyPacketFilter(pkt, iface) {
		return packetJob{}, 0, false
	}

	tc = t.classifyInboundTraffic(data, iface, packetType, destType)

	if packetType == PacketTypeAnnounce {
		if destType == DestTypePlain || destType == DestTypeGroup {
			ifaceProtocolViolation(iface)
			return packetJob{}, 0, false
		}
		if iface != nil {
			if ra, ok := iface.(interface{ ReceivedAnnounceBytes(int) }); ok {
				ra.ReceivedAnnounceBytes(len(data))
			}
		}
	} else if tc == TCPathRequest {
		destHash, _, tag, prOK := parsePathRequestWire(pkt.Data)
		if !prOK {
			ifaceProtocolViolation(iface)
			return packetJob{}, 0, false
		}
		if len(tag) > identity.TruncatedHashLength/8 {
			ifaceProtocolViolation(iface)
		}
		if iface != nil {
			if pr, ok := iface.(interface{ ReceivedPathRequestBytes(int) }); ok {
				pr.ReceivedPathRequestBytes(len(data))
			}
		}
		_ = destHash
	}

	pc := getPacketCopy(len(data))
	copy(pc.buf, data)
	return packetJob{
		pc:         pc,
		iface:      iface,
		packetType: packetType,
		destType:   destType,
		headerType: headerType,
	}, tc, true
}

func (t *Transport) submitInboundJob(job packetJob, tc int, block bool) bool {
	if t.inboundQueues != nil {
		if t.inboundQueues.put(tc, job) {
			return true
		}
		if block {
			if t.inboundQueues.putWait(tc, job, t.done) {
				return true
			}
			putPacketCopy(job.pc)
			return true
		}
		t.shedHandlerOverflow(job.iface)
		return false
	}
	if t.enqueuePacket(job, block) {
		return true
	}
	putPacketCopy(job.pc)
	t.shedHandlerOverflow(job.iface)
	return false
}
