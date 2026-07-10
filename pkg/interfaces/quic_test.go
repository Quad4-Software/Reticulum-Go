// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build !js

package interfaces

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func waitOnline(t *testing.T, iface Interface, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if iface.IsOnline() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s not online within %s", iface.GetName(), timeout)
}

func TestQUICClientServerEcho(t *testing.T) {
	port := freeUDPPort(t)
	srv, err := NewQUICServerInterface("quic_srv", "127.0.0.1", port, QUICServerOptions{})
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

	cli, err := NewQUICClientInterfaceWithRetries("quic_cli", "127.0.0.1", port, true, 20, QUICClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)

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
}

func waitSessions(t *testing.T, srv *QUICServerInterface, n int, timeout time.Duration) {
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

func TestQUICServerToClientEcho(t *testing.T) {
	port := freeUDPPort(t)
	srv, err := NewQUICServerInterface("quic_srv2", "127.0.0.1", port, QUICServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	cli, err := NewQUICClientInterfaceWithRetries("quic_cli2", "127.0.0.1", port, true, 20, QUICClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitSessions(t, srv, 1, 5*time.Second)

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

func TestQUICPeerKeyPinAccept(t *testing.T) {
	port := freeUDPPort(t)
	srv, err := NewQUICServerInterface("quic_pin_srv", "127.0.0.1", port, QUICServerOptions{})
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

	cli, err := NewQUICClientInterfaceWithRetries("quic_pin_cli", "127.0.0.1", port, true, 10, QUICClientOptions{
		PeerKey: pin,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
}

func TestQUICPeerKeyPinReject(t *testing.T) {
	port := freeUDPPort(t)
	srv, err := NewQUICServerInterface("quic_badpin_srv", "127.0.0.1", port, QUICServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	bad := hex.EncodeToString(bytes.Repeat([]byte{0xab}, sha256.Size))
	cli, err := NewQUICClientInterfaceWithRetries("quic_badpin_cli", "127.0.0.1", port, true, 3, QUICClientOptions{
		PeerKey: bad,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	time.Sleep(2 * time.Second)
	if cli.IsOnline() {
		t.Fatal("client should not come online with bad peer_key")
	}
}

func TestQUICFileBackedCert(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	cert, err := generateEphemeralQUICCert()
	if err != nil {
		t.Fatal(err)
	}
	if err := writePEMCertKey(cert, certPath, keyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(certPath); err != nil {
		t.Fatal(err)
	}

	port := freeUDPPort(t)
	srv, err := NewQUICServerInterface("quic_file_srv", "127.0.0.1", port, QUICServerOptions{
		CertFile: certPath,
		KeyFile:  keyPath,
	})
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

	cli, err := NewQUICClientInterfaceWithRetries("quic_file_cli", "127.0.0.1", port, true, 10, QUICClientOptions{
		PeerKey: pin,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)

	var got atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	srv.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Store(append([]byte(nil), data...))
		wg.Done()
	})
	payload := []byte("file-cert")
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
		t.Fatalf("mismatch")
	}
}

func TestQUICIFACRoundTrip(t *testing.T) {
	port := freeUDPPort(t)
	srvCfg := &common.InterfaceConfig{
		Name:        "quic_ifac_srv",
		Type:        "QUICServerInterface",
		Enabled:     true,
		Address:     "127.0.0.1",
		Port:        port,
		NetworkName: "testnet",
		Passphrase:  "secret",
	}
	srvIface, err := NewFromConfig("quic_ifac_srv", srvCfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := srvIface.(*QUICServerInterface)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	cliCfg := &common.InterfaceConfig{
		Name:           "quic_ifac_cli",
		Type:           "QUICClientInterface",
		Enabled:        true,
		TargetHost:     "127.0.0.1",
		TargetPort:     port,
		MaxReconnTries: 10,
		NetworkName:    "testnet",
		Passphrase:     "secret",
	}
	cliIface, err := NewFromConfig("quic_ifac_cli", cliCfg)
	if err != nil {
		t.Fatal(err)
	}
	cli := cliIface.(*QUICClientInterface)
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitSessions(t, srv, 1, 5*time.Second)

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

func TestQUICReconnectAfterServerRestart(t *testing.T) {
	port := freeUDPPort(t)
	srv, err := NewQUICServerInterface("quic_re_srv", "127.0.0.1", port, QUICServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}

	cli, err := NewQUICClientInterfaceWithRetries("quic_re_cli", "127.0.0.1", port, true, -1, QUICClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)

	_ = srv.Stop()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && cli.IsOnline() {
		time.Sleep(50 * time.Millisecond)
	}
	// Wait for UDP port release after listener close.
	time.Sleep(200 * time.Millisecond)

	srv2, err := NewQUICServerInterface("quic_re_srv2", "127.0.0.1", port, QUICServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var startErr error
	for range 20 {
		startErr = srv2.Start()
		if startErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if startErr != nil {
		t.Fatal(startErr)
	}
	defer srv2.Stop()
	waitOnline(t, cli, 15*time.Second)
	waitSessions(t, srv2, 1, 10*time.Second)

	var got atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	srv2.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Store(append([]byte(nil), data...))
		wg.Done()
	})
	payload := []byte("after-reconnect")
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
	case <-time.After(5 * time.Second):
		t.Fatal("timeout after reconnect")
	}
	if !bytes.Equal(got.Load().([]byte), payload) {
		t.Fatalf("got %q", got.Load())
	}
}

func TestQUICConcurrentSend(t *testing.T) {
	port := freeUDPPort(t)
	srv, err := NewQUICServerInterface("quic_conc_srv", "127.0.0.1", port, QUICServerOptions{})
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

	cli, err := NewQUICClientInterfaceWithRetries("quic_conc_cli", "127.0.0.1", port, true, 20, QUICClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitSessions(t, srv, 1, 5*time.Second)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			_ = cli.Send([]byte{0x00, 0x01, byte(i), 0xaa, 0xbb}, "")
		}(i)
	}
	wg.Wait()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && count.Load() < n {
		time.Sleep(20 * time.Millisecond)
	}
	if count.Load() < n {
		t.Fatalf("received %d want %d", count.Load(), n)
	}
}

func TestQUICStopStart(t *testing.T) {
	port := freeUDPPort(t)
	srv, err := NewQUICServerInterface("quic_ss_srv", "127.0.0.1", port, QUICServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatal(err)
	}
	var startErr error
	for range 30 {
		startErr = srv.Start()
		if startErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if startErr != nil {
		t.Fatal(startErr)
	}
	defer srv.Stop()

	cli, err := NewQUICClientInterfaceWithRetries("quic_ss_cli", "127.0.0.1", port, true, 10, QUICClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitSessions(t, srv, 1, 5*time.Second)
	if err := cli.Stop(); err != nil {
		t.Fatal(err)
	}
	cli.Enable()
	if err := cli.Start(); err != nil {
		t.Fatal(err)
	}

	var got atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	srv.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Store(append([]byte(nil), data...))
		wg.Done()
	})
	deadline := time.Now().Add(8 * time.Second)
	var sendErr error
	for time.Now().Before(deadline) {
		sendErr = cli.Send([]byte{0x00, 0x01, 0x72, 0x65}, "")
		if sendErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if sendErr != nil {
		t.Fatalf("send after restart: %v", sendErr)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout after client restart")
	}
	if !bytes.Equal(got.Load().([]byte), []byte{0x00, 0x01, 0x72, 0x65}) {
		t.Fatalf("got %q", got.Load())
	}
}

func TestNewFromConfigQUIC(t *testing.T) {
	port := freeUDPPort(t)
	srv, err := NewFromConfig("cfg_srv", &common.InterfaceConfig{
		Type:    "QUICServerInterface",
		Enabled: true,
		Address: "127.0.0.1",
		Port:    port,
	})
	if err != nil {
		t.Fatal(err)
	}
	if srv.GetType() != common.IFTypeQUIC {
		t.Fatalf("type %v", srv.GetType())
	}
	cli, err := NewFromConfig("cfg_cli", &common.InterfaceConfig{
		Type:           "QUICClientInterface",
		Enabled:        false,
		TargetHost:     "127.0.0.1",
		TargetPort:     port,
		MaxReconnTries: 5,
		PeerKey:        "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899",
		SNI:            "hub.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cli.GetType() != common.IFTypeQUIC {
		t.Fatalf("type %v", cli.GetType())
	}
}
