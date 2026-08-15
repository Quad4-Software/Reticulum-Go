// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"bytes"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestParseRNSTXT(t *testing.T) {
	ep, ok := ParseRNSTXT("rns=udp://127.0.0.1:4242")
	if !ok || ep.Host != "127.0.0.1" || ep.Port != 4242 || ep.Proto != "udp" {
		t.Fatalf("url form: %+v ok=%v", ep, ok)
	}
	ep, ok = ParseRNSTXT("rns proto=udp host=10.0.0.1 port=9999")
	if !ok || ep.Host != "10.0.0.1" || ep.Port != 9999 {
		t.Fatalf("kv form: %+v ok=%v", ep, ok)
	}
	if _, ok := ParseRNSTXT("unrelated"); ok {
		t.Fatal("expected reject")
	}
	if _, ok := ParseRNSTXT("udp:// 00:1"); ok {
		t.Fatal("expected reject whitespace host")
	}
	if _, ok := ParseRNSTXT("rns host= 00 port=1"); ok {
		t.Fatal("expected reject spaced host value")
	}
}

func TestDNSRendezvousEcho(t *testing.T) {
	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	peerPort := peer.LocalAddr().(*net.UDPAddr).Port

	var peerHits atomic.Uint64
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := peer.ReadFromUDP(buf)
			if err != nil {
				return
			}
			peerHits.Add(1)
			_, _ = peer.WriteToUDP(buf[:n], addr)
		}
	}()

	lookup := func(string) ([]string, error) {
		return []string{"rns=udp://127.0.0.1:" + strconv.Itoa(peerPort)}, nil
	}
	di, err := NewDNSRendezvousInterface("dns0", true, DNSRendezvousOptions{
		Domain:          "peers.test",
		ListenAddr:      "127.0.0.1:0",
		ResolveInterval: time.Hour,
		LookupTXT:       lookup,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer di.Stop()

	if di.ActiveTarget() == "" {
		t.Fatal("expected active target after start")
	}

	var got atomic.Pointer[[]byte]
	di.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		cp := append([]byte(nil), data...)
		got.Store(&cp)
	})

	payload := []byte{0x50, 0x53, 0x01}
	if err := di.Send(payload, ""); err != nil {
		t.Fatalf("Send: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if p := got.Load(); p != nil {
			if !bytes.Equal(*p, payload) {
				t.Fatalf("got %x want %x", *p, payload)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout peerHits=%d target=%s rx=%d", peerHits.Load(), di.ActiveTarget(), di.RxPackets)
}
