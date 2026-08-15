// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"quad4/pbt/pkg/pbt"
)

// FuzzReadPCAPUDPPayloads ensures classic pcap parsing never panics on
// arbitrary blobs and never returns a nil error with a nil slice confusion.
func FuzzReadPCAPUDPPayloads(f *testing.F) {
	var good bytes.Buffer
	if err := WritePCAPEthernetUDPv4(&good, []byte{0x01, 0x02, 0x03}, 4242, 4242); err != nil {
		f.Fatal(err)
	}
	f.Add(good.Bytes())
	f.Add([]byte{})
	f.Add([]byte{0xa1, 0xb2, 0xc3, 0xd4})
	f.Add(bytes.Repeat([]byte{0xff}, 64))
	f.Add(append(append([]byte{}, good.Bytes()...), bytes.Repeat([]byte{0x00}, 128)...))
	// Adversarial: wrong magic and empty UDP payload pcap.
	if bad, err := hex.DecodeString("efbeadde0000000000000000000000000000000000000000"); err == nil {
		f.Add(bad)
	}
	if emptyUDP, err := hex.DecodeString("d4c3b2a1020004000000000000000000ffff00000100000000000000000000002a0000002a000000ffffffffffff02000000000108004500001c00000000401100007f0000017f0000011092109200080000"); err == nil {
		f.Add(emptyUDP)
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<18 {
			t.Skip()
		}
		caps, err := ReadPCAPUDPPayloads(bytes.NewReader(raw))
		if err != nil {
			return
		}
		for _, c := range caps {
			if c.InclLen > 16<<20 {
				t.Fatalf("incl len absurd: %d", c.InclLen)
			}
			if c.Payload == nil {
				t.Fatal("pcap capture Payload must be non-nil")
			}
		}
	})
}

func TestPBTPCAPWriteReadRoundTrip(t *testing.T) {
	payload := pbt.Map(
		"payload",
		pbt.SliceOf(pbt.IntRange(0, 255), 0, 512),
		func(xs []int) []byte {
			b := make([]byte, len(xs))
			for i, v := range xs {
				b[i] = byte(v)
			}
			return b
		},
	)
	prop := pbt.ForAll(
		"pcap ethernet udp write then read",
		payload,
		func(data []byte) bool {
			var buf bytes.Buffer
			if err := WritePCAPEthernetUDPv4(&buf, data, 1111, 2222); err != nil {
				return false
			}
			caps, err := ReadPCAPUDPPayloads(bytes.NewReader(buf.Bytes()))
			if err != nil || len(caps) != 1 {
				return false
			}
			c := caps[0]
			return c.FromUDP && c.UDPSport == 1111 && c.UDPDport == 2222 && bytes.Equal(c.Payload, data)
		},
		pbt.WithShrinker(pbt.SliceShrinker[byte]()),
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(44))
}

// FuzzExtractUDPv4 covers link-type branching without requiring a full pcap header.
func FuzzExtractUDPv4(f *testing.F) {
	f.Add([]byte{}, uint32(1))
	f.Add([]byte{0x45}, uint32(101))
	f.Add(bytes.Repeat([]byte{0x00}, 64), uint32(113))
	f.Add(bytes.Repeat([]byte{0xff}, 128), uint32(1))

	f.Fuzz(func(t *testing.T, frame []byte, linkType uint32) {
		if len(frame) > 1<<14 {
			t.Skip()
		}
		_, _, _, _ = extractUDPv4(frame, linkType)
	})
}

func TestExtractUDPv4RejectsUDPLengthBelowHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := WritePCAPEthernetUDPv4(&buf, []byte{1, 2, 3}, 1, 2); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	// Patch UDP length field inside the single Ethernet frame.
	udpLenOff := 24 + 16 + 14 + 20 + 4
	if len(raw) <= udpLenOff+1 {
		t.Fatalf("pcap too short: %d", len(raw))
	}
	binary.BigEndian.PutUint16(raw[udpLenOff:], 4)
	caps, err := ReadPCAPUDPPayloads(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadPCAPUDPPayloads: %v", err)
	}
	if len(caps) != 0 {
		t.Fatalf("expected invalid UDP length to be skipped, got %d captures", len(caps))
	}
}
