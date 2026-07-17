// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

// fakeSAMForDial serves HELLO, SESSION CREATE, and STREAM CONNECT.
// When failConnect is true, STREAM CONNECT returns CANT_REACH_PEER.
func fakeSAMForDial(t *testing.T, failConnect bool, sessionCreates *atomic.Int32) (addr string, cleanup func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
					return
				}
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					switch {
					case strings.HasPrefix(line, "HELLO"):
						_, _ = c.Write([]byte("HELLO REPLY RESULT=OK\n"))
					case strings.HasPrefix(line, "SESSION CREATE"):
						if sessionCreates != nil {
							sessionCreates.Add(1)
						}
						if !strings.Contains(line, "i2cp.leaseSetEncType=6,4") {
							_, _ = c.Write([]byte("SESSION STATUS RESULT=I2P_ERROR MESSAGE=\"missing options\"\n"))
							return
						}
						_, _ = c.Write([]byte("SESSION STATUS RESULT=OK\n"))
					case strings.HasPrefix(line, "STREAM CONNECT"):
						if failConnect {
							_, _ = c.Write([]byte("STREAM STATUS RESULT=CANT_REACH_PEER MESSAGE=\"LeaseSet not found\"\n"))
							return
						}
						_, _ = c.Write([]byte("STREAM STATUS RESULT=OK\n"))
						buf := make([]byte, 1)
						_, _ = c.Read(buf)
						return
					default:
						return
					}
				}
			}(conn)
		}
	})
	return ln.Addr().String(), func() {
		close(done)
		_ = ln.Close()
		wg.Wait()
	}
}

func TestI2PPeerDialFailStaysOffline(t *testing.T) {
	addr, cleanup := fakeSAMForDial(t, true, nil)
	defer cleanup()

	parent, err := NewI2PInterface("i2p_offline", &common.InterfaceConfig{
		Type:          "I2PInterface",
		Enabled:       true,
		I2PSAMAddress: addr,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.controller.Stop()

	peer := NewI2PInterfacePeer(parent, "i2p_offline to dest",
		"abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst.b32.i2p", 1, parent.cfg)
	defer peer.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if peer.LastError() != "" && !peer.IsOnline() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if peer.IsOnline() {
		t.Fatal("peer must stay offline when STREAM CONNECT fails")
	}
	if peer.LastError() == "" {
		t.Fatal("expected LastError after failed dial")
	}
	if !strings.Contains(peer.LastError(), "CANT_REACH_PEER") {
		t.Fatalf("LastError: %q", peer.LastError())
	}
}

func TestI2PPeerDialSuccessGoesOnline(t *testing.T) {
	var creates atomic.Int32
	addr, cleanup := fakeSAMForDial(t, false, &creates)
	defer cleanup()

	parent, err := NewI2PInterface("i2p_online", &common.InterfaceConfig{
		Type:          "I2PInterface",
		Enabled:       true,
		I2PSAMAddress: addr,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.controller.Stop()

	peer := NewI2PInterfacePeer(parent, "i2p_online to dest",
		"abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst.b32.i2p", 1, parent.cfg)
	defer peer.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if peer.IsOnline() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !peer.IsOnline() {
		t.Fatalf("peer should be online after STREAM CONNECT OK last=%q", peer.LastError())
	}
	if creates.Load() < 1 {
		t.Fatal("expected at least one SESSION CREATE")
	}
}

func TestI2PPeerReconnectOpensNewSession(t *testing.T) {
	var creates atomic.Int32
	addr, cleanup := fakeSAMForDial(t, false, &creates)
	defer cleanup()

	parent, err := NewI2PInterface("i2p_reconn", &common.InterfaceConfig{
		Type:          "I2PInterface",
		Enabled:       true,
		I2PSAMAddress: addr,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.controller.Stop()

	peer := NewI2PInterfacePeer(parent, "i2p_reconn to dest",
		"abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst.b32.i2p", 3, parent.cfg)
	defer peer.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if peer.IsOnline() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !peer.IsOnline() {
		t.Fatal("peer never came online")
	}
	firstCreates := creates.Load()

	peer.Mutex.RLock()
	conn := peer.conn
	peer.Mutex.RUnlock()
	if conn != nil {
		_ = conn.Close()
	}

	deadline = time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if creates.Load() > firstCreates && peer.IsOnline() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if creates.Load() <= firstCreates {
		t.Fatalf("expected new SESSION CREATE on reconnect first=%d now=%d", firstCreates, creates.Load())
	}
}

func TestI2PPeerDoubleStopSafe(t *testing.T) {
	addr, cleanup := fakeSAMForDial(t, false, nil)
	defer cleanup()
	parent, err := NewI2PInterface("i2p_dblstop", &common.InterfaceConfig{
		Type:          "I2PInterface",
		Enabled:       true,
		I2PSAMAddress: addr,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.controller.Stop()
	peer := NewI2PInterfacePeer(parent, "i2p_dblstop to dest",
		"abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst.b32.i2p", 1, parent.cfg)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !peer.IsOnline() {
		time.Sleep(20 * time.Millisecond)
	}
	if err := peer.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := peer.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if peer.IsOnline() {
		t.Fatal("peer should be offline after Stop")
	}
}

func TestI2PPeerStopCancelsDial(t *testing.T) {
	// SAM that hangs on STREAM CONNECT until cancelled.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Go(func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(conn net.Conn) {
				defer wg.Done()
				defer conn.Close()
				br := bufio.NewReader(conn)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					switch {
					case strings.HasPrefix(line, "HELLO"):
						_, _ = conn.Write([]byte("HELLO REPLY RESULT=OK\n"))
					case strings.HasPrefix(line, "SESSION CREATE"):
						_, _ = conn.Write([]byte("SESSION STATUS RESULT=OK\n"))
					case strings.HasPrefix(line, "STREAM CONNECT"):
						select {
						case <-done:
						case <-time.After(30 * time.Second):
						}
						return
					}
				}
			}(c)
		}
	})

	parent, err := NewI2PInterface("i2p_cancel", &common.InterfaceConfig{
		Type:          "I2PInterface",
		Enabled:       true,
		I2PSAMAddress: ln.Addr().String(),
	}, nil)
	if err != nil {
		_ = ln.Close()
		t.Fatal(err)
	}
	peer := NewI2PInterfacePeer(parent, "i2p_cancel to dest",
		"abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst.b32.i2p", 1, parent.cfg)
	time.Sleep(200 * time.Millisecond)
	_ = peer.Stop()
	_ = parent.Stop()
	close(done)
	_ = ln.Close()
	wg.Wait()
	if peer.IsOnline() {
		t.Fatal("peer must stay offline when Stop cancels dial")
	}
}

func TestI2PPeerMaxReconnectExhausted(t *testing.T) {
	addr, cleanup := fakeSAMForDial(t, true, nil)
	defer cleanup()
	parent, err := NewI2PInterface("i2p_maxre", &common.InterfaceConfig{
		Type:          "I2PInterface",
		Enabled:       true,
		I2PSAMAddress: addr,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.controller.Stop()

	// maxReconnect=1: initial fail then reconnect path with one attempt.
	peer := NewI2PInterfacePeer(parent, "i2p_maxre to dest",
		"abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst.b32.i2p", 1, parent.cfg)
	defer peer.Stop()

	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		if !peer.In && !peer.Out {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if peer.IsOnline() {
		t.Fatal("peer should not be online after exhausted reconnects")
	}
}
