// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"net"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/ifac"
)

func BenchmarkTCPSendIFAC(b *testing.B) {
	id, err := ifac.New(16, "bench-net", "bench-pass")
	if err != nil {
		b.Fatal(err)
	}
	server, client := net.Pipe()
	tc := newTestTCPClient()
	tc.SetIFAC(id)
	tc.Mutex.Lock()
	tc.conn = client
	tc.MTU = DefaultMTU
	tc.txFrame = make([]byte, 0, DefaultMTU*2+4)
	tc.Mutex.Unlock()
	go func() {
		buf := make([]byte, DefaultMTU*2+4)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	raw := make([]byte, 512)
	raw[0] = 0x40
	raw[1] = 0x01
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := tc.Send(raw, ""); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTCPSendNoIFAC(b *testing.B) {
	server, client := net.Pipe()
	tc := newTestTCPClient()
	tc.Mutex.Lock()
	tc.conn = client
	tc.MTU = DefaultMTU
	tc.txFrame = make([]byte, 0, DefaultMTU*2+4)
	tc.Mutex.Unlock()
	go func() {
		buf := make([]byte, DefaultMTU*2+4)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	raw := make([]byte, 512)
	raw[0] = 0x40
	raw[1] = 0x01
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := tc.Send(raw, ""); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBackboneSendIFAC(b *testing.B) {
	benchmarkBackboneSendPayload(b, true)
}

func BenchmarkBackboneSendNoIFAC(b *testing.B) {
	benchmarkBackboneSendPayload(b, false)
}

func benchmarkBackboneSendPayload(b *testing.B, withIFAC bool) {
	backbone.Shutdown()
	hub, err := backbone.Init(backbone.BackendGo)
	if err != nil {
		b.Fatal(err)
	}
	defer backbone.Shutdown()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	server, err := NewBackboneInterface("server", &common.InterfaceConfig{
		Enabled: true,
		Address: "127.0.0.1",
		Port:    port,
	}, hub, nil)
	if err != nil {
		b.Fatal(err)
	}
	if err := server.Start(); err != nil {
		b.Fatal(err)
	}
	defer server.Stop()

	client, err := NewBackboneClientInterface("client", &common.InterfaceConfig{
		Enabled:    true,
		TargetHost: "127.0.0.1",
		TargetPort: port,
	}, hub)
	if err != nil {
		b.Fatal(err)
	}
	if withIFAC {
		id, err := ifac.New(16, "bench-net", "bench-pass")
		if err != nil {
			b.Fatal(err)
		}
		client.SetIFAC(id)
	}
	if err := client.Start(); err != nil {
		b.Fatal(err)
	}
	defer client.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if client.IsOnline() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !client.IsOnline() {
		b.Fatal("client offline")
	}

	payload := make([]byte, 512)
	payload[0] = 0x40
	payload[1] = 0x01
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := client.Send(payload, ""); err != nil {
			b.Fatal(err)
		}
	}
}
