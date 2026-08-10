// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common_test

import (
	"bytes"
	"sync/atomic"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/ifac"
)

func oracleMaskedPacket(t *testing.T, netname, netkey string) []byte {
	t.Helper()
	id, err := ifac.New(16, netname, netkey)
	if err != nil {
		t.Fatalf("ifac.New: %v", err)
	}
	raw := bytes.Repeat([]byte{0x42}, 64)
	raw[0] = 0x01
	raw[1] = 0x00
	masked, err := id.Mask(raw)
	if err != nil {
		t.Fatalf("Mask: %v", err)
	}
	return masked
}

// Guarantee: IFAC-configured ProcessIncoming never invokes PacketCallback on policy violations.
func TestOracleIFACInboundNeverCallbacksOnViolation(t *testing.T) {
	health.Default.Reset()
	iface := common.NewBaseInterface("oracle-ifac", common.IFTypeUDP, true)
	goodID, err := ifac.New(16, "alpha", "beta")
	if err != nil {
		t.Fatalf("ifac.New good: %v", err)
	}
	iface.SetIFAC(goodID)

	var callbackCalls atomic.Int32
	iface.SetPacketCallback(func([]byte, common.NetworkInterface) {
		callbackCalls.Add(1)
	})

	masked := oracleMaskedPacket(t, "alpha", "beta")
	iface.ProcessIncoming(masked)
	if callbackCalls.Load() != 1 {
		t.Fatalf("valid IFAC callback calls=%d want 1", callbackCalls.Load())
	}

	wrongCred := oracleMaskedPacket(t, "gamma", "delta")
	iface.ProcessIncoming(wrongCred)
	if callbackCalls.Load() != 1 {
		t.Fatalf("wrong credentials callback calls=%d want 1", callbackCalls.Load())
	}

	corrupt := oracleMaskedPacket(t, "alpha", "beta")
	corrupt[5] ^= 0x01
	iface.ProcessIncoming(corrupt)
	if callbackCalls.Load() != 1 {
		t.Fatalf("corrupt IFAC callback calls=%d want 1", callbackCalls.Load())
	}

	unflagged := []byte{0x01, 0x02, 0x03, 0x04}
	iface.ProcessIncoming(unflagged)
	if callbackCalls.Load() != 1 {
		t.Fatalf("missing IFAC flag callback calls=%d want 1", callbackCalls.Load())
	}

	snap := health.Default.SnapshotIface("oracle-ifac")
	if snap.IFACFail.Total < 3 {
		t.Fatalf("ifac_fail=%d want >= 3", snap.IFACFail.Total)
	}
	if snap.RxOK.Total != 1 {
		t.Fatalf("rx_ok=%d want 1", snap.RxOK.Total)
	}
}

// Guarantee: interface without IFAC drops IFAC-flagged packets without callback.
func TestOracleIFACFlagWithoutConfigNeverCallbacks(t *testing.T) {
	health.Default.Reset()
	iface := common.NewBaseInterface("oracle-no-ifac", common.IFTypeUDP, true)

	var callbackCalls atomic.Int32
	iface.SetPacketCallback(func([]byte, common.NetworkInterface) {
		callbackCalls.Add(1)
	})

	flagged := []byte{0x80, 0x01, 0x02, 0x03}
	iface.ProcessIncoming(flagged)
	if callbackCalls.Load() != 0 {
		t.Fatalf("callback calls=%d want 0 for flagged packet without IFAC", callbackCalls.Load())
	}

	clean := []byte{0x01, 0x02, 0x03}
	iface.ProcessIncoming(clean)
	if callbackCalls.Load() != 1 {
		t.Fatalf("callback calls=%d want 1 for clean packet", callbackCalls.Load())
	}

	snap := health.Default.SnapshotIface("oracle-no-ifac")
	if snap.IFACFail.Total != 1 {
		t.Fatalf("ifac_fail=%d want 1", snap.IFACFail.Total)
	}
}
