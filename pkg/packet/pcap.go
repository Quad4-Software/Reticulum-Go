// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// CapturedPacket is one frame extracted from a classic pcap file.
type CapturedPacket struct {
	Index    int
	TSSec    uint32
	TSUSec   uint32
	InclLen  uint32
	OrigLen  uint32
	LinkType uint32
	Payload  []byte
	UDPSport uint16
	UDPDport uint16
	FromUDP  bool
}

// ReadPCAPUDPPayloads reads a little-endian classic pcap and yields UDP payloads
// for IPv4 Ethernet or Linux cooked captures. Non-UDP packets are skipped.
func ReadPCAPUDPPayloads(r io.Reader) ([]CapturedPacket, error) {
	hdr := make([]byte, 24)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, fmt.Errorf("pcap global header: %w", err)
	}
	magic := binary.LittleEndian.Uint32(hdr[0:4])
	var order binary.ByteOrder = binary.LittleEndian
	switch magic {
	case 0xa1b2c3d4, 0xa1b23c4d:
		order = binary.LittleEndian
	case 0xd4c3b2a1, 0x4d3cb2a1:
		order = binary.BigEndian
	default:
		return nil, fmt.Errorf("unsupported pcap magic %08x", magic)
	}
	linkType := order.Uint32(hdr[20:24])

	var out []CapturedPacket
	pktHdr := make([]byte, 16)
	idx := 0
	for {
		if _, err := io.ReadFull(r, pktHdr); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return out, fmt.Errorf("pcap packet header: %w", err)
		}
		tsSec := order.Uint32(pktHdr[0:4])
		tsUsec := order.Uint32(pktHdr[4:8])
		incl := order.Uint32(pktHdr[8:12])
		orig := order.Uint32(pktHdr[12:16])
		if incl > 16<<20 {
			return out, fmt.Errorf("pcap record too large: %d", incl)
		}
		body := make([]byte, incl)
		if _, err := io.ReadFull(r, body); err != nil {
			return out, fmt.Errorf("pcap record body: %w", err)
		}
		idx++
		cp := CapturedPacket{
			Index:    idx,
			TSSec:    tsSec,
			TSUSec:   tsUsec,
			InclLen:  incl,
			OrigLen:  orig,
			LinkType: linkType,
		}
		payload, sport, dport, ok := extractUDPv4(body, linkType)
		if !ok {
			continue
		}
		cp.Payload = payload
		cp.UDPSport = sport
		cp.UDPDport = dport
		cp.FromUDP = true
		out = append(out, cp)
	}
	return out, nil
}

func extractUDPv4(frame []byte, linkType uint32) (payload []byte, sport, dport uint16, ok bool) {
	off := 0
	switch linkType {
	case 1: // DLT_EN10MB
		if len(frame) < 14 {
			return nil, 0, 0, false
		}
		ethType := binary.BigEndian.Uint16(frame[12:14])
		off = 14
		if ethType == 0x8100 {
			if len(frame) < 18 {
				return nil, 0, 0, false
			}
			ethType = binary.BigEndian.Uint16(frame[16:18])
			off = 18
		}
		if ethType != 0x0800 {
			return nil, 0, 0, false
		}
	case 113: // DLT_LINUX_SLL
		if len(frame) < 16 {
			return nil, 0, 0, false
		}
		proto := binary.BigEndian.Uint16(frame[14:16])
		off = 16
		if proto != 0x0800 {
			return nil, 0, 0, false
		}
	case 101: // DLT_RAW
		off = 0
	default:
		// Try raw IPv4 if it looks like one.
		if len(frame) > 0 && frame[0]>>4 == 4 {
			off = 0
		} else {
			return nil, 0, 0, false
		}
	}
	if len(frame) < off+20 {
		return nil, 0, 0, false
	}
	ip := frame[off:]
	if ip[0]>>4 != 4 {
		return nil, 0, 0, false
	}
	ihl := int(ip[0]&0x0f) * 4
	if ihl < 20 || len(ip) < ihl+8 {
		return nil, 0, 0, false
	}
	if ip[9] != 17 {
		return nil, 0, 0, false
	}
	udp := ip[ihl:]
	sport = binary.BigEndian.Uint16(udp[0:2])
	dport = binary.BigEndian.Uint16(udp[2:4])
	ulen := int(binary.BigEndian.Uint16(udp[4:6]))
	if ulen < 8 {
		return nil, 0, 0, false
	}
	if len(udp) < ulen {
		ulen = len(udp)
	}
	return append([]byte{}, udp[8:ulen]...), sport, dport, true
}

// WritePCAPEthernetUDPv4 writes a minimal little-endian pcap with one Ethernet/IPv4/UDP packet.
func WritePCAPEthernetUDPv4(w io.Writer, payload []byte, srcPort, dstPort uint16) error {
	gh := make([]byte, 24)
	binary.LittleEndian.PutUint32(gh[0:4], 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(gh[4:6], 2)
	binary.LittleEndian.PutUint16(gh[6:8], 4)
	binary.LittleEndian.PutUint32(gh[16:20], 65535)
	binary.LittleEndian.PutUint32(gh[20:24], 1) // EN10MB
	if _, err := w.Write(gh); err != nil {
		return err
	}

	udpLen := 8 + len(payload)
	ipLen := 20 + udpLen
	frameLen := 14 + ipLen
	// Cap at uint16 so IP/UDP length fields and the uint32 pcap header stay in range
	// on both 32-bit and 64-bit ints (0xffffffff is not a valid 32-bit int constant).
	if udpLen > 0xffff || ipLen > 0xffff {
		return errors.New("pcap: frame too large for UDP/IPv4 headers")
	}
	frame := make([]byte, frameLen)
	// Ethernet: dest, src, type IPv4
	copy(frame[0:6], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	copy(frame[6:12], []byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x01})
	frame[12], frame[13] = 0x08, 0x00
	ip := frame[14:]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(ipLen)) // #nosec G115 -- bounded above
	ip[8] = 64
	ip[9] = 17
	copy(ip[12:16], []byte{127, 0, 0, 1})
	copy(ip[16:20], []byte{127, 0, 0, 1})
	// Skip IP checksum for synthetic capture tools that do not validate it.
	udp := ip[20:]
	binary.BigEndian.PutUint16(udp[0:2], srcPort)
	binary.BigEndian.PutUint16(udp[2:4], dstPort)
	binary.BigEndian.PutUint16(udp[4:6], uint16(udpLen)) // #nosec G115 -- bounded above
	copy(udp[8:], payload)

	ph := make([]byte, 16)
	binary.LittleEndian.PutUint32(ph[8:12], uint32(frameLen))  // #nosec G115 -- bounded above
	binary.LittleEndian.PutUint32(ph[12:16], uint32(frameLen)) // #nosec G115 -- bounded above
	if _, err := w.Write(ph); err != nil {
		return err
	}
	_, err := w.Write(frame)
	return err
}
