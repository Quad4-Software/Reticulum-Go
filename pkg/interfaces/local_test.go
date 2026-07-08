// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"net"
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
