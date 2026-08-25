// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"net"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
)

func TestLocalServerClientHDLCRoundTrip(t *testing.T) {
	readEnd, writeEnd := net.Pipe()
	lc := newLocalClientFromConn("spawned", readEnd, nil, false)
	lc.In = true
	lc.MTU = 262144

	got := make(chan []byte, 1)
	lc.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got <- append([]byte(nil), data...)
	})
	go lc.readLoop()

	frame := []byte{HDLCFlag, 0x42, 0x43, HDLCFlag}
	if _, err := writeEnd.Write(frame); err != nil {
		t.Fatalf("write frame: %v", err)
	}

	select {
	case rcv := <-got:
		if !bytes.Equal(rcv, []byte{0x42, 0x43}) {
			t.Fatalf("payload = %x", rcv)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for HDLC frame")
	}
	_ = lc.Stop()
	_ = writeEnd.Close()
}

func TestLocalInterfaceConfigDefaults(t *testing.T) {
	iface, err := NewLocalFromConfig("loc", &common.InterfaceConfig{
		Type:    "LocalInterface",
		Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lc := iface.(*LocalClientInterface)
	if lc.targetPort != defaultLocalPort {
		t.Fatalf("port = %d, want %d", lc.targetPort, defaultLocalPort)
	}
}

func TestLocalServerInterfaceConfig(t *testing.T) {
	port := freeTCPPort(t)
	ctx := &FromConfigContext{
		BackboneHub: backbone.Get(),
		SpawnLocal:  func(*LocalClientInterface) {},
	}
	iface, err := NewLocalFromConfig("srv", &common.InterfaceConfig{
		Type:    "LocalServerInterface",
		Enabled: true,
		Port:    port,
	}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	ls := iface.(*LocalServerInterface)
	if ls.bindPort != port {
		t.Fatalf("bindPort = %d, want %d", ls.bindPort, port)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestLocalSpawnedClientNameTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	serverConnCh := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		serverConnCh <- conn
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	serverConn := <-serverConnCh
	defer serverConn.Close()

	ls := &LocalServerInterface{useUnix: false}
	name := localSpawnedClientName(serverConn, ls)
	want := strconv.Itoa(serverConn.RemoteAddr().(*net.TCPAddr).Port)
	if name != want {
		t.Fatalf("name = %q, want %q", name, want)
	}
}

func TestLocalSpawnedClientNameUnix(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("abstract unix sockets are Linux-only")
	}
	ls := &LocalServerInterface{useUnix: true, socketPath: "testinst"}
	ls.clients.Store(2)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	_ = clientConn.Close()

	name := localSpawnedClientName(serverConn, ls)
	if name != "2@rns/testinst" {
		t.Fatalf("name = %q, want 2@rns/testinst", name)
	}
}

func TestLocalServerUnixMultipleClientsRegister(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("abstract unix sockets are Linux-only")
	}

	var mu sync.Mutex
	registered := make(map[string]struct{})

	spawn := func(client *LocalClientInterface) {
		name := client.GetName()
		mu.Lock()
		if _, exists := registered[name]; exists {
			mu.Unlock()
			t.Errorf("duplicate client name %q", name)
			_ = client.Stop()
			return
		}
		registered[name] = struct{}{}
		mu.Unlock()
		client.SetDisconnectHooks(func() {
			mu.Lock()
			delete(registered, name)
			mu.Unlock()
		}, nil)
	}

	ls, err := NewLocalServerInterface(0, "multitest", true, spawn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ls.Start(); err != nil {
		t.Fatal(err)
	}
	defer ls.Stop()

	dial := func() net.Conn {
		t.Helper()
		conn, err := net.Dial("unix", "@rns/multitest")
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}

	conn1 := dial()
	defer conn1.Close()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(registered) != 1 {
		t.Fatalf("registered count = %d, want 1", len(registered))
	}
	if _, ok := registered["0@rns/multitest"]; !ok {
		t.Fatalf("registered = %v, want 0@rns/multitest", registered)
	}
	mu.Unlock()

	conn2 := dial()
	defer conn2.Close()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(registered) != 2 {
		t.Fatalf("registered count = %d, want 2", len(registered))
	}
	if _, ok := registered["1@rns/multitest"]; !ok {
		t.Fatalf("registered = %v, want 1@rns/multitest", registered)
	}
	mu.Unlock()

	_ = conn1.Close()
	time.Sleep(100 * time.Millisecond)

	conn3 := dial()
	defer conn3.Close()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if _, ok := registered["2@rns/multitest"]; !ok {
		t.Fatalf("registered = %v, want 2@rns/multitest after reconnect", registered)
	}
	mu.Unlock()
}
