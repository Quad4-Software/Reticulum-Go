// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

// PacketClass groups ingress packets for priority shedding.
type PacketClass int

const (
	// ClassUnknown uses default shed behavior.
	ClassUnknown PacketClass = iota
	// ClassShedFirst drops before prefer-keep traffic when over the trip line.
	ClassShedFirst
	// ClassPreferKeep allows up to 2x the trip line before shed unless cool-down or memory shed.
	ClassPreferKeep
)

func (c PacketClass) preferKeep() bool {
	return c == ClassPreferKeep
}

// AdmitOpts carries optional ingress metadata for protect gates.
type AdmitOpts struct {
	Bitrate int64
	Class   PacketClass
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
func scaledFloors(bitrate int64, floorPPS, floorBPS, maxPPS, maxBPS float64) (float64, float64) {
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
	pps := floorPPS * ratio
	bps := floorBPS * ratio
	if pps > maxPPS*0.9 {
		pps = maxPPS * 0.9
	}
	if bps > maxBPS*0.9 {
		bps = maxBPS * 0.9
	}
	return pps, bps
}
