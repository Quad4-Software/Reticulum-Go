// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/ifac"
)

func TestNewUDPInterface(t *testing.T) {
	validAddr := "127.0.0.1:0" // Use port 0 for OS to assign a free port
	validTarget := "127.0.0.1:8080"
	invalidAddr := "invalid-address"

	t.Run("ValidConfig", func(t *testing.T) {
		ui, err := NewUDPInterface("udpValid", validAddr, validTarget, true)
		if err != nil {
			t.Fatalf("NewUDPInterface failed with valid config: %v", err)
		}
		if ui == nil {
			t.Fatal("NewUDPInterface returned nil interface with valid config")
		}
		if ui.GetName() != "udpValid" {
			t.Errorf("GetName() = %s; want udpValid", ui.GetName())
		}
		if ui.GetType() != common.IFTypeUDP {
			t.Errorf("GetType() = %v; want %v", ui.GetType(), common.IFTypeUDP)
		}
		if ui.targetAddr.String() != validTarget {
			t.Errorf("Resolved targetAddr = %s; want %s", ui.targetAddr.String(), validTarget)
		}
		if !ui.Enabled { // BaseInterface field
			t.Error("Interface not enabled by default when requested")
		}
		if ui.IsOnline() { // Should be offline initially
			t.Error("Interface online initially")
		}
	})

	t.Run("ValidConfigNoTarget", func(t *testing.T) {
		ui, err := NewUDPInterface("udpNoTarget", validAddr, "", true)
		if err != nil {
			t.Fatalf("NewUDPInterface failed with valid config (no target): %v", err)
		}
		if ui == nil {
			t.Fatal("NewUDPInterface returned nil interface with valid config (no target)")
		}
		if ui.targetAddr != nil {
			t.Errorf("targetAddr = %v; want nil", ui.targetAddr)
		}
	})

	t.Run("InvalidAddress", func(t *testing.T) {
		_, err := NewUDPInterface("udpInvalidAddr", invalidAddr, validTarget, true)
		if err == nil {
			t.Error("NewUDPInterface succeeded with invalid address")
		}
	})

	t.Run("InvalidTarget", func(t *testing.T) {
		_, err := NewUDPInterface("udpInvalidTarget", validAddr, invalidAddr, true)
		if err == nil {
			t.Error("NewUDPInterface succeeded with invalid target address")
		}
	})
}

func TestUDPInterfaceState(t *testing.T) {
	// Basic state tests are covered by BaseInterface tests
	addr := "127.0.0.1:0"
	ui, _ := NewUDPInterface("udpState", addr, "", true)

	if ui.conn != nil {
		t.Error("conn field is not nil before Start()")
	}

	// We don't call Start() here because it requires actual network binding
	// Testing Send requires Start() and a listener, which is too complex for unit tests here

	// Test Detach
	ui.Detach()
	if !ui.IsDetached() {
		t.Error("IsDetached() is false after Detach()")
	}

	// Further tests on Send/ProcessOutgoing/readLoop would require mocking net.UDPConn
	// or setting up a local listener.
}

func TestUDPProcessIncomingCountsRx(t *testing.T) {
	ui, err := NewUDPInterface("udpRx", "127.0.0.1:0", "", true)
	if err != nil {
		t.Fatalf("NewUDPInterface: %v", err)
	}
	called := false
	ui.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		called = true
		if len(data) != 4 {
			t.Errorf("callback len=%d want 4", len(data))
		}
	})
	payload := []byte{0x00, 0x01, 0x02, 0x03}
	ui.ProcessIncoming(payload)
	if !called {
		t.Fatal("callback not invoked")
	}
	if ui.GetRxBytes() != uint64(len(payload)) {
		t.Fatalf("RxBytes=%d want %d", ui.GetRxBytes(), len(payload))
	}
	if ui.GetRxPackets() != 1 {
		t.Fatalf("RxPackets=%d want 1", ui.GetRxPackets())
	}
}

// TestUDPProcessIncomingDefersIFACWhenRegistered ensures UDP leaves IFAC
// bytes intact when transport has requested deferred inbound IFAC. Stripping
// here would make preprocessInboundPacket see a missing IFAC flag and drop
// otherwise-valid Python (or Go) IFAC announces.
func TestUDPProcessIncomingDefersIFACWhenRegistered(t *testing.T) {
	ui, err := NewUDPInterface("udpDeferIFAC", "127.0.0.1:0", "", true)
	if err != nil {
		t.Fatalf("NewUDPInterface: %v", err)
	}
	id, err := ifac.New(16, "defer-net", "defer-pass")
	if err != nil {
		t.Fatalf("ifac.New: %v", err)
	}
	ui.SetIFAC(id)
	ui.SetDeferInboundIFAC(true)

	plain := []byte{0x01, 0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x11, 0x22}
	masked, err := id.Mask(plain)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	if masked[0]&0x80 == 0 {
		t.Fatal("masked packet missing IFAC flag")
	}

	var got []byte
	ui.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got = append([]byte(nil), data...)
	})
	ui.ProcessIncoming(masked)
	if len(got) == 0 {
		t.Fatal("callback not invoked")
	}
	if len(got) != len(masked) {
		t.Fatalf("deferred path altered length got=%d want=%d", len(got), len(masked))
	}
	if got[0]&0x80 == 0 {
		t.Fatal("deferred path stripped IFAC flag before transport")
	}
	for i := range got {
		if got[i] != masked[i] {
			t.Fatalf("deferred path mutated byte %d", i)
		}
	}
}

func TestUDPProcessIncomingAppliesIFACWhenNotDeferred(t *testing.T) {
	ui, err := NewUDPInterface("udpApplyIFAC", "127.0.0.1:0", "", true)
	if err != nil {
		t.Fatalf("NewUDPInterface: %v", err)
	}
	id, err := ifac.New(16, "apply-net", "apply-pass")
	if err != nil {
		t.Fatalf("ifac.New: %v", err)
	}
	ui.SetIFAC(id)

	plain := []byte{0x01, 0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x11, 0x22}
	masked, err := id.Mask(plain)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}

	var got []byte
	ui.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got = append([]byte(nil), data...)
	})
	ui.ProcessIncoming(masked)
	if len(got) == 0 {
		t.Fatal("callback not invoked")
	}
	if len(got) != len(plain) {
		t.Fatalf("standalone IFAC path length got=%d want=%d", len(got), len(plain))
	}
	if got[0]&0x80 != 0 {
		t.Fatal("standalone path left IFAC flag set")
	}
	for i := range plain {
		if got[i] != plain[i] {
			t.Fatalf("standalone unmask mismatch at %d", i)
		}
	}
}
