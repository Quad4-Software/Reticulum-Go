// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"bytes"
	"testing"
)

func TestPrepareHT1BufferMatchesPack(t *testing.T) {
	dest := bytes.Repeat([]byte{0xAB}, TruncatedHashLength)
	payload := []byte("ht1-payload")

	viaPack := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		TransportType:   0,
		Context:         ContextNone,
		ContextFlag:     FlagUnset,
		Hops:            0,
		DestinationType: DestinationLink,
		DestinationHash: dest,
		Data:            payload,
	}
	if err := viaPack.Pack(); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	viaPrep := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		TransportType:   0,
		Context:         ContextNone,
		ContextFlag:     FlagUnset,
		Hops:            0,
		DestinationType: DestinationLink,
		DestinationHash: dest,
	}
	buf, err := viaPrep.PrepareHT1Buffer(dest, len(payload))
	if err != nil {
		t.Fatalf("PrepareHT1Buffer: %v", err)
	}
	copy(buf, payload)
	if err := viaPrep.CommitPacked(); err != nil {
		t.Fatalf("CommitPacked: %v", err)
	}

	if !bytes.Equal(viaPack.Raw, viaPrep.Raw) {
		t.Fatalf("raw mismatch\npack %x\nprep %x", viaPack.Raw, viaPrep.Raw)
	}
	if !bytes.Equal(viaPack.GetHash(), viaPrep.GetHash()) {
		t.Fatalf("hash mismatch")
	}
	if HeaderType1Overhead != 2+TruncatedHashLength+1 {
		t.Fatalf("HeaderType1Overhead=%d", HeaderType1Overhead)
	}
}

func TestPrepareHT1BufferRejectsOversize(t *testing.T) {
	p := &Packet{HeaderType: HeaderType1, DestinationType: DestinationLink}
	if _, err := p.PrepareHT1Buffer(bytes.Repeat([]byte{0x01}, TruncatedHashLength), MTU); err == nil {
		t.Fatal("expected MTU error")
	}
}
