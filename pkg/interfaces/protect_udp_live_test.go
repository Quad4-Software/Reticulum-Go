// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/protect"
)

func freeUDPPortLocal(t *testing.T) int {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func startUDPPair(t *testing.T, nameA, nameB string) (a, b *UDPInterface, cleanup func()) {
	t.Helper()
	portA := freeUDPPortLocal(t)
	portB := freeUDPPortLocal(t)
	var err error
	a, err = NewUDPInterface(nameA, fmt.Sprintf("127.0.0.1:%d", portA), fmt.Sprintf("127.0.0.1:%d", portB), true)
	if err != nil {
		t.Fatal(err)
	}
	b, err = NewUDPInterface(nameB, fmt.Sprintf("127.0.0.1:%d", portB), fmt.Sprintf("127.0.0.1:%d", portA), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		_ = a.Stop()
		t.Fatal(err)
	}
	cleanup = func() {
		_ = a.Stop()
		_ = b.Stop()
	}
	return a, b, cleanup
}

// TestLiveUDPQuietNoFalsePositive sends real datagrams over loopback UDP ifaces.
func TestLiveUDPQuietNoFalsePositive(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModePrevent,
		MaxPPS:       protect.DefaultMaxPPS,
		FloorPPS:     protect.DefaultFloorPPS,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	protect.SetDefault(e)

	a, b, cleanup := startUDPPair(t, "live_a", "live_b")
	defer cleanup()

	var got atomic.Int64
	b.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Add(1)
	})

	payload := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
	for range 40 {
		if err := a.ProcessOutgoing(payload); err != nil {
			t.Fatalf("send: %v", err)
		}
		time.Sleep(40 * time.Millisecond)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && got.Load() < 30 {
		time.Sleep(20 * time.Millisecond)
	}
	if got.Load() < 30 {
		t.Fatalf("expected quiet deliveries got=%d", got.Load())
	}
	if e.TripCount(protect.ReasonPPS) != 0 || e.TripCount(protect.ReasonBPS) != 0 {
		t.Fatalf("false positive trips pps=%d bps=%d warn=%q",
			e.TripCount(protect.ReasonPPS), e.TripCount(protect.ReasonBPS), buf.String())
	}
}

// TestLiveUDPFloodPreventSheds sheds on the live iface ProcessIncoming path.
// Flood is driven in-process so OS UDP loss cannot hide protect trips (Darwin CI).
func TestLiveUDPFloodPreventSheds(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:            protect.ModePrevent,
		MaxPPS:          40,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: true,
	})
	protect.SetDefault(e)

	a, b, cleanup := startUDPPair(t, "flood_a", "flood_b")
	defer cleanup()
	_ = a

	var got atomic.Int64
	b.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Add(1)
	})

	payload := make([]byte, 64)
	const n = 400
	for range n {
		b.ProcessIncoming(payload)
	}
	if got.Load() >= int64(n) {
		t.Fatalf("prevent flood should shed got=%d", got.Load())
	}
	if e.TripCount(protect.ReasonPPS) == 0 {
		t.Fatal("expected pps trips on live flood")
	}
}

// TestLiveUDPDetectFloodDelivers confirms detect never sheds on the live iface path.
func TestLiveUDPDetectFloodDelivers(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:            protect.ModeDetect,
		MaxPPS:          20,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: true,
	})
	protect.SetDefault(e)

	a, b, cleanup := startUDPPair(t, "det_a", "det_b")
	defer cleanup()
	_ = a

	var got atomic.Int64
	b.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Add(1)
	})
	payload := make([]byte, 32)
	const n = 120
	// Drive ProcessIncoming on the live iface so UDP loss cannot mask protect behavior.
	for range n {
		b.ProcessIncoming(payload)
	}
	if got.Load() != int64(n) {
		t.Fatalf("detect must deliver all got=%d want=%d", got.Load(), n)
	}
	if e.TripCount(protect.ReasonPPS) == 0 {
		t.Fatal("detect should still count pps trips")
	}
}

// TestLiveUDPOnRealNICQuietNoFalsePositive binds a real non-loopback NIC when present.
func TestLiveUDPOnRealNICQuietNoFalsePositive(t *testing.T) {
	ip := firstNonLoopbackIPv4(t)
	if ip == "" {
		t.Skip("no non-loopback IPv4 interface available")
	}
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModePrevent,
		MaxPPS:       protect.DefaultMaxPPS,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	protect.SetDefault(e)

	portA := freeUDPPortOn(t, ip)
	portB := freeUDPPortOn(t, ip)
	a, err := NewUDPInterface("nic_a", fmt.Sprintf("%s:%d", ip, portA), fmt.Sprintf("%s:%d", ip, portB), true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewUDPInterface("nic_b", fmt.Sprintf("%s:%d", ip, portB), fmt.Sprintf("%s:%d", ip, portA), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Start(); err != nil {
		t.Skipf("cannot bind %s: %v", ip, err)
	}
	defer a.Stop()
	if err := b.Start(); err != nil {
		t.Skipf("cannot bind peer on %s: %v", ip, err)
	}
	defer b.Stop()

	var got atomic.Int64
	b.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Add(1)
	})
	payload := []byte("rns-protect-nic")
	for range 25 {
		if err := a.ProcessOutgoing(payload); err != nil {
			t.Fatalf("send: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && got.Load() < 20 {
		time.Sleep(20 * time.Millisecond)
	}
	if got.Load() < 20 {
		t.Fatalf("nic quiet deliveries got=%d", got.Load())
	}
	if e.TripCount(protect.ReasonPPS) != 0 {
		t.Fatalf("nic false positive trips=%d", e.TripCount(protect.ReasonPPS))
	}
}

func freeUDPPortOn(t *testing.T, ip string) int {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(ip), Port: 0})
	if err != nil {
		t.Skipf("listen on %s: %v", ip, err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func firstNonLoopbackIPv4(t *testing.T) string {
	t.Helper()
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil || !ip.IsGlobalUnicast() {
				continue
			}
			return ip.String()
		}
	}
	return ""
}
