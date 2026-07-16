// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live Go-Go DNS rendezvous: in-process DNS TXT server plus UDP peer.
// Set RUN_LIVE_INTEROP=1.

//go:build !js

package interop

import (
	"bytes"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
)

func TestLiveInteropDNSRendezvousGoGo(t *testing.T) {
	liveOrSkip(t)

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

	dnsUDP, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer dnsUDP.Close()
	dnsPort := dnsUDP.LocalAddr().(*net.UDPAddr).Port

	mux := dns.NewServeMux()
	mux.HandleFunc("peers.live.test.", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		txt := fmt.Sprintf("rns=udp://127.0.0.1:%d", peerPort)
		m.Answer = append(m.Answer, &dns.TXT{
			Hdr: dns.RR_Header{Name: "peers.live.test.", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 5},
			Txt: []string{txt},
		})
		_ = w.WriteMsg(m)
	})
	go func() { _ = dns.ActivateAndServe(nil, dnsUDP, mux) }()

	lookup := func(name string) ([]string, error) {
		c := new(dns.Client)
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(name), dns.TypeTXT)
		in, _, err := c.Exchange(m, fmt.Sprintf("127.0.0.1:%d", dnsPort))
		if err != nil {
			return nil, err
		}
		var out []string
		for _, a := range in.Answer {
			if t, ok := a.(*dns.TXT); ok {
				out = append(out, t.Txt...)
			}
		}
		return out, nil
	}

	di, err := interfaces.NewDNSRendezvousInterface("live-dns", true, interfaces.DNSRendezvousOptions{
		Domain:          "peers.live.test",
		ListenAddr:      "127.0.0.1:0",
		ResolveInterval: time.Hour,
		LookupTXT:       lookup,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer di.Stop()

	var got atomic.Pointer[[]byte]
	di.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		cp := append([]byte(nil), data...)
		got.Store(&cp)
	})

	payload := []byte{0x44, 0x4e, 0x53, 0x21} // "DNS!" header bit 7 clear (no IFAC)
	if err := di.Send(payload, ""); err != nil {
		t.Fatalf("Send: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p := got.Load(); p != nil {
			if !bytes.Equal(*p, payload) {
				t.Fatalf("got %x want %x", *p, payload)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout waiting for DNS rendezvous echo")
}
