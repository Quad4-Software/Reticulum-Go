// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package interfaces

import (
	"bytes"
	"net"
	"runtime"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestNewBackboneInterfaceDefaults(t *testing.T) {
	cfg := &common.InterfaceConfig{
		Enabled:     true,
		Address:     "127.0.0.1",
		Port:        4242,
		KISSFraming: false,
	}
	bi, err := NewBackboneInterface("bb", cfg)
	if err != nil {
		t.Fatalf("NewBackboneInterface: %v", err)
	}
	if bi == nil {
		t.Fatal("nil interface")
	}
	if bi.MTU != 1048576 {
		t.Errorf("MTU = %d, want 1048576", bi.MTU)
	}
	if bi.Bitrate != 1000000000 {
		t.Errorf("Bitrate = %d, want 1e9", bi.Bitrate)
	}
	if bi.Type != common.IFTypeBackbone {
		t.Errorf("Type = %v, want IFTypeBackbone", bi.Type)
	}
}

func TestNewBackboneInterfaceNoPort(t *testing.T) {
	cfg := &common.InterfaceConfig{Enabled: true, Address: "127.0.0.1"}
	_, err := NewBackboneInterface("bb", cfg)
	if err == nil {
		t.Fatal("expected error for missing port")
	}
}

func TestNewBackboneInterfaceFallbackPort(t *testing.T) {
	cfg := &common.InterfaceConfig{
		Enabled:    true,
		Address:    "127.0.0.1",
		TargetPort: 9999,
	}
	bi, err := NewBackboneInterface("bb", cfg)
	if err != nil {
		t.Fatalf("NewBackboneInterface: %v", err)
	}
	if bi.bindPort != 9999 {
		t.Errorf("bindPort = %d, want 9999", bi.bindPort)
	}
}

func TestNewBackboneInterfaceResolveDevice(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "js" {
		t.Skip("skipping interface resolution on this platform")
	}

	// Use loopback interface, which exists on every platform.
	lo, err := net.InterfaceByName("lo")
	if err != nil {
		t.Skipf("loopback interface not found: %v", err)
	}
	loAddrs, err := lo.Addrs()
	if err != nil || len(loAddrs) == 0 {
		t.Skip("loopback has no addresses")
	}

	cfg := &common.InterfaceConfig{
		Enabled:   true,
		Interface: "lo",
		Port:      4242,
	}
	bi, err := NewBackboneInterface("bb", cfg)
	if err != nil {
		t.Fatalf("NewBackboneInterface: %v", err)
	}
	if bi.bindAddr == "" {
		t.Fatal("expected bindAddr to be resolved from lo")
	}
}

func TestNewBackboneInterfaceBadDevice(t *testing.T) {
	cfg := &common.InterfaceConfig{
		Enabled:   true,
		Interface: "nonexistent0",
		Port:      4242,
	}
	_, err := NewBackboneInterface("bb", cfg)
	if err == nil {
		t.Fatal("expected error for bad interface name")
	}
}

func TestBackboneInterfaceHDLCRoundTrip(t *testing.T) {
	packets := [][]byte{
		{0x01, 0x02, 0x03},
		make([]byte, 100),
		{HDLCFlag, HDLCEsc, 0xAB},
		make([]byte, 1024),
	}
	packets[1][0] = 0x7E
	packets[3][len(packets[3])-1] = 0x7D

	for _, pkt := range packets {
		escaped := escapeHDLC(pkt)
		decoded := unescapeHDLC(escaped)
		if !bytes.Equal(decoded, pkt) {
			t.Errorf("HDLC round-trip failed:\n in=%x\nout=%x", pkt, decoded)
		}
	}
}

func TestBackboneInterfaceStartStop(t *testing.T) {
	cfg := &common.InterfaceConfig{
		Enabled: true,
		Address: "127.0.0.1",
		Port:    4242,
	}
	bi, err := NewBackboneInterface("bb", cfg)
	if err != nil {
		t.Fatalf("NewBackboneInterface: %v", err)
	}
	if err := bi.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !bi.IsOnline() {
		t.Error("expected online after Start")
	}
	if err := bi.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func FuzzHDLCEscapeUnescape(f *testing.F) {
	f.Add([]byte{0x01, 0x02, 0x03})
	f.Add([]byte{HDLCFlag, HDLCEsc, 0xAB, 0xCD})
	f.Add([]byte{})
	f.Add(make([]byte, 256))
	f.Fuzz(func(t *testing.T, data []byte) {
		escaped := escapeHDLC(data)
		// Escaped must not contain raw flag or esc bytes
		for _, b := range escaped {
			if b == HDLCFlag {
				t.Fatal("raw HDLCFlag in escaped data")
			}
		}
		decoded := unescapeHDLC(escaped)
		if !bytes.Equal(decoded, data) {
			t.Fatalf("round-trip failed: in=%x out=%x", data, decoded)
		}
	})
}

func BenchmarkEscapeHDLC(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = escapeHDLC(data)
	}
}

func BenchmarkUnescapeHDLC(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i)
	}
	escaped := escapeHDLC(data)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = unescapeHDLC(escaped)
	}
}
