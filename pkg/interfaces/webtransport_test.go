// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build !js

package interfaces

import (
	"bytes"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func freeWTPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func waitWTSessions(t *testing.T, srv *WebTransportServerInterface, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if srv.SessionCount() >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("want %d sessions, have %d", n, srv.SessionCount())
}

func listenWTPort(t *testing.T, srv *WebTransportServerInterface) int {
	t.Helper()
	addr := srv.ListenAddr()
	if addr == nil {
		t.Fatal("nil listen addr")
	}
	ua, ok := addr.(*net.UDPAddr)
	if !ok {
		t.Fatalf("unexpected addr %T", addr)
	}
	return ua.Port
}

func TestWebTransportDatagramEcho(t *testing.T) {
	port := freeWTPort(t)
	srv, err := NewWebTransportServerInterface("wt_dg_srv", "127.0.0.1", port, "/rns", WebTransportServerOptions{
		TransportMode: "datagram",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	srv.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Store(append([]byte(nil), data...))
		wg.Done()
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = listenWTPort(t, srv)

	cli, err := NewWebTransportClientInterfaceWithRetries("wt_dg_cli", "127.0.0.1", port, "/rns", true, 20, WebTransportClientOptions{
		TransportMode: "datagram",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitWTSessions(t, srv, 1, 5*time.Second)

	payload := []byte{0x01, 0x02, 0x03, 0xaa, 0xbb}
	if err := cli.Send(payload, ""); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for server packet")
	}
	recv, _ := got.Load().([]byte)
	if !bytes.Equal(recv, payload) {
		t.Fatalf("got %x want %x", recv, payload)
	}
	if cli.DatagramsTX.Load() < 1 || srv.DatagramsRX.Load() < 1 {
		t.Fatalf("datagram stats: cliTX=%d srvRX=%d", cli.DatagramsTX.Load(), srv.DatagramsRX.Load())
	}
}

func TestWebTransportServerToClientDatagram(t *testing.T) {
	port := freeWTPort(t)
	srv, err := NewWebTransportServerInterface("wt_dg_srv2", "127.0.0.1", port, "", WebTransportServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = listenWTPort(t, srv)

	cli, err := NewWebTransportClientInterfaceWithRetries("wt_dg_cli2", "127.0.0.1", port, "", true, 20, WebTransportClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitWTSessions(t, srv, 1, 5*time.Second)

	var got atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	cli.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Store(append([]byte(nil), data...))
		wg.Done()
	})

	payload := []byte{0x10, 0x20, 0x30}
	if err := srv.Send(payload, ""); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for client packet")
	}
	recv, _ := got.Load().([]byte)
	if !bytes.Equal(recv, payload) {
		t.Fatalf("got %x want %x", recv, payload)
	}
}

func TestWebTransportStreamEcho(t *testing.T) {
	port := freeWTPort(t)
	srv, err := NewWebTransportServerInterface("wt_st_srv", "127.0.0.1", port, "/rns", WebTransportServerOptions{
		TransportMode: "stream",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	srv.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Store(append([]byte(nil), data...))
		wg.Done()
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = listenWTPort(t, srv)

	cli, err := NewWebTransportClientInterfaceWithRetries("wt_st_cli", "127.0.0.1", port, "/rns", true, 20, WebTransportClientOptions{
		TransportMode: "stream",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitWTSessions(t, srv, 1, 5*time.Second)

	payload := []byte{0x01, 0x02, 0x7e, 0x7d}
	if err := cli.Send(payload, ""); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for stream packet")
	}
	recv, _ := got.Load().([]byte)
	if !bytes.Equal(recv, payload) {
		t.Fatalf("got %x want %x", recv, payload)
	}
	if cli.StreamFramesTX.Load() < 1 || srv.StreamFramesRX.Load() < 1 {
		t.Fatalf("stream stats: cliTX=%d srvRX=%d", cli.StreamFramesTX.Load(), srv.StreamFramesRX.Load())
	}
}

func TestWebTransportDualMode(t *testing.T) {
	port := freeWTPort(t)
	srv, err := NewWebTransportServerInterface("wt_dual_srv", "127.0.0.1", port, "/rns", WebTransportServerOptions{
		TransportMode: "dual",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	srv.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Store(append([]byte(nil), data...))
		wg.Done()
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = listenWTPort(t, srv)

	cli, err := NewWebTransportClientInterfaceWithRetries("wt_dual_cli", "127.0.0.1", port, "/rns", true, 20, WebTransportClientOptions{
		TransportMode: "dual",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitWTSessions(t, srv, 1, 5*time.Second)

	payload := []byte("dual-dg")
	if err := cli.Send(payload, ""); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout dual datagram")
	}
	if !bytes.Equal(got.Load().([]byte), payload) {
		t.Fatalf("got %q", got.Load())
	}
}

func TestWebTransportIFACRoundTrip(t *testing.T) {
	port := freeWTPort(t)
	srvCfg := &common.InterfaceConfig{
		Name:          "wt_ifac_srv",
		Type:          "WebTransportServerInterface",
		Enabled:       true,
		Address:       "127.0.0.1",
		Port:          port,
		Path:          "/rns",
		TransportMode: "datagram",
		NetworkName:   "testnet",
		Passphrase:    "secret",
	}
	srvIface, err := NewFromConfig("wt_ifac_srv", srvCfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := srvIface.(*WebTransportServerInterface)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = listenWTPort(t, srv)

	cliCfg := &common.InterfaceConfig{
		Name:           "wt_ifac_cli",
		Type:           "WebTransportClientInterface",
		Enabled:        true,
		TargetHost:     "127.0.0.1",
		TargetPort:     port,
		Path:           "/rns",
		TransportMode:  "datagram",
		MaxReconnTries: 10,
		NetworkName:    "testnet",
		Passphrase:     "secret",
	}
	cliIface, err := NewFromConfig("wt_ifac_cli", cliCfg)
	if err != nil {
		t.Fatal(err)
	}
	cli := cliIface.(*WebTransportClientInterface)
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitWTSessions(t, srv, 1, 5*time.Second)

	if srv.GetIFAC() == nil || cli.GetIFAC() == nil {
		t.Fatal("IFAC not applied")
	}

	var got atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	srv.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Store(append([]byte(nil), data...))
		wg.Done()
	})
	payload := []byte{
		0x00, 0x01,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x00, 0x41, 0x42, 0x43, 0x44,
	}
	if err := cli.Send(payload, ""); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	if !bytes.Equal(got.Load().([]byte), payload) {
		t.Fatalf("IFAC round-trip mismatch: %x", got.Load())
	}
}

func TestWebTransportConcurrentSend(t *testing.T) {
	port := freeWTPort(t)
	srv, err := NewWebTransportServerInterface("wt_conc_srv", "127.0.0.1", port, "/rns", WebTransportServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var count atomic.Int64
	srv.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		count.Add(1)
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = listenWTPort(t, srv)

	cli, err := NewWebTransportClientInterfaceWithRetries("wt_conc_cli", "127.0.0.1", port, "/rns", true, 20, WebTransportClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitWTSessions(t, srv, 1, 5*time.Second)

	const n = 50
	deadline := time.Now().Add(20 * time.Second)
	seq := 0
	// Datagrams can be dropped under load. Keep sending until n are received or timeout.
	for time.Now().Before(deadline) && count.Load() < int64(n) {
		need := n - int(count.Load())
		if need > 10 {
			need = 10
		}
		var wg sync.WaitGroup
		wg.Add(need)
		for i := 0; i < need; i++ {
			seq++
			go func(i int) {
				defer wg.Done()
				_ = cli.Send([]byte{0x00, 0x01, byte(i), 0xaa, 0xbb}, "")
			}(seq)
		}
		wg.Wait()
		time.Sleep(50 * time.Millisecond)
	}
	if count.Load() < int64(n) {
		t.Fatalf("received %d want %d", count.Load(), n)
	}
}

func TestWebTransportPeerKeyPin(t *testing.T) {
	port := freeWTPort(t)
	srv, err := NewWebTransportServerInterface("wt_pin_srv", "127.0.0.1", port, "/rns", WebTransportServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pin, err := srv.LeafSPKIPinHex()
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = listenWTPort(t, srv)

	cli, err := NewWebTransportClientInterfaceWithRetries("wt_pin_cli", "127.0.0.1", port, "/rns", true, 10, WebTransportClientOptions{
		PeerKey: pin,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
}

func TestWebTransportBadMode(t *testing.T) {
	_, err := NewWebTransportClientInterface("bad", "127.0.0.1", 1, "/rns", false, WebTransportClientOptions{
		TransportMode: "udp",
	})
	if err == nil {
		t.Fatal("expected error for bad transport_mode")
	}
}

func TestNewFromConfigWebTransport(t *testing.T) {
	port := freeWTPort(t)
	srv, err := NewFromConfig("cfg_wt_srv", &common.InterfaceConfig{
		Type:          "WebTransportServerInterface",
		Enabled:       true,
		Address:       "127.0.0.1",
		Port:          port,
		Path:          "/mesh",
		TransportMode: "stream",
	})
	if err != nil {
		t.Fatal(err)
	}
	if srv.GetType() != common.IFTypeWebTransport {
		t.Fatalf("type %v", srv.GetType())
	}
	cli, err := NewFromConfig("cfg_wt_cli", &common.InterfaceConfig{
		Type:           "WebTransportClientInterface",
		Enabled:        false,
		TargetHost:     "127.0.0.1",
		TargetPort:     port,
		Path:           "/mesh",
		TransportMode:  "dual",
		MaxReconnTries: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cli.GetType() != common.IFTypeWebTransport {
		t.Fatalf("type %v", cli.GetType())
	}
}
