// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestDecodeFrameRoundTripData(t *testing.T) {
	dest := bytes.Repeat([]byte{0x11}, TruncatedHashLength)
	p := NewPacket(DestinationSingle, []byte("hello"), PacketTypeData, ContextNone, PropagationBroadcast, HeaderType1, nil, false, 0)
	p.DestinationHash = dest
	if err := p.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	fr := DecodeFrame(p.Raw)
	if !fr.OK {
		t.Fatalf("decode: %s", fr.Error)
	}
	if fr.PacketTypeName != "DATA" || fr.ContextName != "NONE" {
		t.Fatalf("names type=%s ctx=%s", fr.PacketTypeName, fr.ContextName)
	}
	if fr.DestinationHash != hex.EncodeToString(dest) {
		t.Fatalf("dest hash %s", fr.DestinationHash)
	}
}

func TestPCAPRoundTripUDPPayload(t *testing.T) {
	dest := bytes.Repeat([]byte{0x22}, TruncatedHashLength)
	p := NewPacket(DestinationSingle, []byte("pcap"), PacketTypeData, ContextNone, PropagationBroadcast, HeaderType1, nil, false, 0)
	p.DestinationHash = dest
	if err := p.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	var buf bytes.Buffer
	if err := WritePCAPEthernetUDPv4(&buf, p.Raw, 4242, 4242); err != nil {
		t.Fatalf("write pcap: %v", err)
	}
	caps, err := ReadPCAPUDPPayloads(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("read pcap: %v", err)
	}
	if len(caps) != 1 {
		t.Fatalf("caps=%d", len(caps))
	}
	fr := DecodeFrame(caps[0].Payload)
	if !fr.OK || fr.PacketTypeName != "DATA" {
		t.Fatalf("frame %#v", fr)
	}
}

func TestPacketTypeAndContextNames(t *testing.T) {
	if PacketTypeName(PacketTypeAnnounce) != "ANNOUNCE" {
		t.Fatal("announce name")
	}
	if ContextName(ContextPathResponse) != "PATH_RESPONSE" {
		t.Fatal("path response name")
	}
	if ContextName(ContextLRProof) != "LRPROOF" {
		t.Fatal("lrproof name")
	}
}
