// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestDNSRendezvousRaceStopSend(t *testing.T) {
	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	peerPort := peer.LocalAddr().(*net.UDPAddr).Port
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := peer.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = peer.WriteToUDP(buf[:n], addr)
		}
	}()

	lookup := func(string) ([]string, error) {
		return []string{"rns=udp://127.0.0.1:" + strconv.Itoa(peerPort)}, nil
	}
	di, err := NewDNSRendezvousInterface("dns_race", true, DNSRendezvousOptions{
		Domain:          "peers.race.test",
		ListenAddr:      "127.0.0.1:0",
		ResolveInterval: 50 * time.Millisecond,
		LookupTXT:       lookup,
	})
	if err != nil {
		t.Fatal(err)
	}

	var n atomic.Int32
	di.SetPacketCallback(func([]byte, common.NetworkInterface) { n.Add(1) })

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := range 80 {
			_ = di.Send([]byte{0x50, byte(i)}, "")
			time.Sleep(time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		for range 40 {
			_ = di.ForceResolve()
			time.Sleep(2 * time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(40 * time.Millisecond)
		_ = di.Stop()
	}()
	wg.Wait()
	_ = n.Load()
}
