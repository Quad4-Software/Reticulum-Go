// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"runtime"
	"testing"
	"time"
)

func TestDNSRendezvousNoGoroutineLeak(t *testing.T) {
	runtime.GC()
	baseline := runtime.NumGoroutine()

	lookup := func(string) ([]string, error) {
		return []string{"rns=udp://127.0.0.1:9"}, nil
	}
	for range 40 {
		di, err := NewDNSRendezvousInterface("dns_leak", true, DNSRendezvousOptions{
			Domain:          "peers.leak.test",
			ListenAddr:      "127.0.0.1:0",
			ResolveInterval: time.Hour,
			LookupTXT:       lookup,
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = di.Send([]byte{0x50, 0x01}, "")
		if err := di.Stop(); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(80 * time.Millisecond)
	runtime.GC()
	final := runtime.NumGoroutine()
	if final > baseline+8 {
		t.Errorf("possible goroutine leak: baseline=%d final=%d", baseline, final)
	}
}
