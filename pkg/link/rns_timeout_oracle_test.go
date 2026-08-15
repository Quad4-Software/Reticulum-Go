// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"math"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
)

func TestOracleLinkConstantsMatchPythonRNS(t *testing.T) {
	if EstablishmentTimeoutPerHop != 6 {
		t.Fatalf("ESTABLISHMENT_TIMEOUT_PER_HOP=%v want 6", EstablishmentTimeoutPerHop)
	}
	if Keepalive != 360 {
		t.Fatalf("KEEPALIVE=%v want 360", Keepalive)
	}
	if StaleTime != 720 {
		t.Fatalf("STALE_TIME=%v want 720", StaleTime)
	}
	if KeepaliveTimeoutFactor != 4 {
		t.Fatalf("KEEPALIVE_TIMEOUT_FACTOR=%v want 4", KeepaliveTimeoutFactor)
	}
	if TrafficTimeoutFactor != 6 {
		t.Fatalf("TRAFFIC_TIMEOUT_FACTOR=%v want 6", TrafficTimeoutFactor)
	}
	if ECPubSize != 64 {
		t.Fatalf("ECPUBSIZE=%d want 64", ECPubSize)
	}
	if KeepaliveRequestByte != 0xFF || KeepaliveResponseByte != 0xFE {
		t.Fatalf("keepalive bytes request=%#02x response=%#02x", KeepaliveRequestByte, KeepaliveResponseByte)
	}
}

func TestOracleLinkMDUMatchesPythonFormula(t *testing.T) {
	l := &Link{mtu: packet.MTU}
	l.updateMDU()
	if l.mdu != 431 {
		t.Fatalf("Link.MDU=%d want 431", l.mdu)
	}
}

func TestOracleEstablishmentTimeoutAddsFirstHopAndPerHop(t *testing.T) {
	cases := []struct {
		firstHop float64
		hops     int
		want     float64
	}{
		{6, 1, 12},
		{38, 1, 44},
		{38, 3, 56},
		{806, 1, 812},
	}
	for _, tc := range cases {
		hops := max(tc.hops, 1)
		got := tc.firstHop + float64(hops)*EstablishmentTimeoutPerHop
		if math.Abs(got-tc.want) > 1e-12 {
			t.Fatalf("first=%v hops=%d timeout=%v want %v", tc.firstHop, tc.hops, got, tc.want)
		}
		gotDur := time.Duration((tc.firstHop+float64(hops)*EstablishmentTimeoutPerHop)*float64(time.Second)) + 6*time.Second
		if math.Abs(gotDur.Seconds()-(tc.want+6)) > 1e-9 {
			t.Fatalf("app window=%s want %vs", gotDur, tc.want+6)
		}
	}
}

func TestOracleSignallingBytesCarryMTUAndMode(t *testing.T) {
	raw := signallingBytes(packet.MTU, ModeAES256CBC)
	if len(raw) != LinkMTUSize {
		t.Fatalf("signalling len=%d", len(raw))
	}
	mtu := (int(raw[0]&0x1F) << 16) | (int(raw[1]) << 8) | int(raw[2])
	if mtu != packet.MTU {
		t.Fatalf("signalling MTU=%d want %d", mtu, packet.MTU)
	}
	mode := (raw[0] & ModeByteMask) >> 5
	if mode != ModeAES256CBC {
		t.Fatalf("mode=%d want AES256CBC", mode)
	}
}

func TestOracleDefaultPerHopMatchesCommon(t *testing.T) {
	if EstablishmentTimeoutPerHop != common.EstablishTimeout {
		t.Fatalf("link per-hop %v common %v", EstablishmentTimeoutPerHop, common.EstablishTimeout)
	}
}
