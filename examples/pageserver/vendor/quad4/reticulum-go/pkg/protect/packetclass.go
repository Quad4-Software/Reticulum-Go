// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

// PacketClass groups ingress packets for priority shedding.
type PacketClass int

const (
	// ClassUnknown is data and path-request traffic. It uses prefer-keep
	// leniency so a busy public mesh does not drop discovery under load.
	ClassUnknown PacketClass = iota
	// ClassShedFirst drops before prefer-keep traffic when over the trip line.
	ClassShedFirst
	// ClassPreferKeep rides out bursts over the trip line up to 2x, or the advertised bitrate if higher.
	ClassPreferKeep
)

func (c PacketClass) preferKeep() bool {
	return c != ClassShedFirst
}

// AdmitOpts carries optional ingress metadata for protect gates.
type AdmitOpts struct {
	Bitrate int64
	Class   PacketClass
	// PeerKey identifies the remote sender on a shared local interface (for
	// example a remote address string). When set, the sender gets its own
	// fair-share sub-bucket independent of the interface-wide aggregate, so
	// one hostile peer cannot exhaust the whole interface budget and cool
	// down every other peer sharing it. Leave empty for interfaces that are
	// inherently single-peer, where the interface bucket already is the
	// peer bucket.
	PeerKey string
}

// wire header bit masks aligned with pkg/transport/wire.go.
const (
	wireDestTypeMask   byte = 0x0C
	wireDestTypeShift  uint = 2
	wirePacketTypeMask byte = 0x03

	wireDestTypeLink   byte = 0x03
	wirePacketAnnounce byte = 0x01
	wirePacketLink     byte = 0x02
)

// PeekPacketClass inspects the first header byte of a Reticulum wire packet.
func PeekPacketClass(data []byte) PacketClass {
	if len(data) < 1 {
		return ClassUnknown
	}
	h := data[0]
	packetType := h & wirePacketTypeMask
	destType := (h & wireDestTypeMask) >> wireDestTypeShift
	switch packetType {
	case wirePacketLink:
		return ClassPreferKeep
	case wirePacketAnnounce:
		return ClassShedFirst
	}
	if destType == wireDestTypeLink {
		return ClassPreferKeep
	}
	return ClassUnknown
}

// refBitrateBPS is the reference link rate for scaling adaptive floors.
const refBitrateBPS = 1_000_000

// scaledFloors raises floors on high-bitrate interfaces and lowers them on slow radios.
func scaledFloors(bitrate int64, floorPPS, floorBPS, maxPPS, maxBPS float64) (pps, bps float64) {
	if bitrate <= 0 {
		return floorPPS, floorBPS
	}
	ratio := float64(bitrate) / refBitrateBPS
	if ratio < 0.05 {
		ratio = 0.05
	}
	if ratio > 50 {
		ratio = 50
	}
	pps = floorPPS * ratio
	bps = floorBPS * ratio
	if pps > maxPPS*0.9 {
		pps = maxPPS * 0.9
	}
	if bps > maxBPS*0.9 {
		bps = maxBPS * 0.9
	}
	return pps, bps
}

// preferKeepMinPacket is the smallest frame used to turn a bit-rate into a
// packet-rate cap so a resource stream of typical link packets is not treated
// as a flood after a quiet baseline.
const preferKeepMinPacket = 64

// preferKeepCaps is the prefer-keep leniency ceiling. It is at least 2x the
// adaptive trip line, and on a rated interface it also reaches the advertised
// byte rate (capped at maxPPS/maxBPS) so a UDP or TCP resource transfer can
// fill the guessed link after a quiet learn.
func preferKeepCaps(tripPPS, tripBPS, maxPPS, maxBPS float64, bitrate int64) (pps, bps float64) {
	pps = tripPPS * 2
	bps = tripBPS * 2
	if bitrate > 0 {
		linkBPS := float64(bitrate) / 8
		if linkBPS > maxBPS {
			linkBPS = maxBPS
		}
		if linkBPS > bps {
			bps = linkBPS
		}
		linkPPS := linkBPS / preferKeepMinPacket
		if linkPPS > maxPPS {
			linkPPS = maxPPS
		}
		if linkPPS > pps {
			pps = linkPPS
		}
	}
	return pps, bps
}
