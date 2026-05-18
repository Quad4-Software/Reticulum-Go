// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"crypto/aes"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

func TestUpdateMDU_ClampsAtPacketMTU(t *testing.T) {
	cases := []struct {
		name        string
		negotiated  int
		wantClamped int
	}{
		{"under_packet_mtu", 250, 250},
		{"at_packet_mtu", packet.MTU, packet.MTU},
		{"tcp_default", 1064, packet.MTU},
		{"hwmtu_pi", 1196, packet.MTU},
		{"oversize_int24", 0xFFFFFF, packet.MTU},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := &Link{}
			l.mtu = tc.negotiated
			l.updateMDU()
			if l.mtu != tc.wantClamped {
				t.Fatalf("mtu=%d after clamp, want %d", l.mtu, tc.wantClamped)
			}
		})
	}
}

func TestUpdateMDU_ProducesMDUThatFitsInPacketMTU(t *testing.T) {
	negotiated := []int{200, 500, 553, 1064, 1196, 4096, 0xFFFFFF}

	const headerType1Size = 19
	const ivSize = aes.BlockSize
	const hmacSize = 32
	const maxPad = aes.BlockSize

	for _, mtu := range negotiated {
		l := &Link{}
		l.mtu = mtu
		l.updateMDU()

		if l.mdu <= 0 {
			t.Fatalf("mtu=%d produced non-positive mdu=%d", mtu, l.mdu)
		}

		paddedPlaintext := ((l.mdu + aes.BlockSize) / aes.BlockSize) * aes.BlockSize
		if paddedPlaintext < l.mdu+1 {
			paddedPlaintext = l.mdu + maxPad
		}
		worstWire := headerType1Size + ivSize + paddedPlaintext + hmacSize

		if worstWire > packet.MTU {
			t.Fatalf("mtu=%d -> mdu=%d -> worst raw=%d > packet.MTU=%d",
				mtu, l.mdu, worstWire, packet.MTU)
		}
	}
}

func TestUpdateMDU_NegativeFallback(t *testing.T) {
	l := &Link{}
	l.mtu = 10
	l.updateMDU()

	if l.mdu != common.DefaultMTU/15 {
		t.Fatalf("expected fallback mdu=%d, got %d", common.DefaultMTU/15, l.mdu)
	}
}

func TestUpdateMDU_StableAcrossRepeatedCalls(t *testing.T) {
	l := &Link{}
	l.mtu = 1196
	l.updateMDU()
	mtu1, mdu1 := l.mtu, l.mdu

	for range 5 {
		l.updateMDU()
	}

	if l.mtu != mtu1 || l.mdu != mdu1 {
		t.Fatalf("updateMDU is not idempotent: mtu %d->%d, mdu %d->%d",
			mtu1, l.mtu, mdu1, l.mdu)
	}
}

func TestUpdateMDU_DoesNotRaiseLowMTU(t *testing.T) {
	l := &Link{}
	l.mtu = 200
	l.updateMDU()
	if l.mtu != 200 {
		t.Fatalf("low mtu was modified: got %d, want 200", l.mtu)
	}
}

func TestSignallingBytes_HighMTUEncodes(t *testing.T) {
	const maxEncodableMTU = (1 << 21) - 1
	cases := []int{200, 500, 1064, 1196, maxEncodableMTU}
	for _, mtu := range cases {
		b := signallingBytes(mtu, ModeAES256CBC)
		if len(b) != LinkMTUSize {
			t.Fatalf("len mismatch: %d", len(b))
		}
		got := (int(b[0]&0x1F) << 16) | (int(b[1]) << 8) | int(b[2])
		if got != mtu {
			t.Fatalf("encode/decode mtu mismatch: got %d, want %d", got, mtu)
		}
		mode := (b[0] & ModeByteMask) >> 5
		if mode != ModeAES256CBC {
			t.Fatalf("mode mismatch: got %d, want %d", mode, ModeAES256CBC)
		}
	}
}
