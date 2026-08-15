package transport

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
)

type stubLink struct{}

func (s *stubLink) GetStatus() byte         { return StatusActive }
func (s *stubLink) GetRTT() float64         { return 0 }
func (s *stubLink) RTT() float64            { return 0 }
func (s *stubLink) GetLinkID() []byte       { return []byte("stub") }
func (s *stubLink) Send(data []byte) any    { return nil }
func (s *stubLink) Resend(packet any) error { return nil }
func (s *stubLink) SetPacketTimeout(packet any, callback func(any), timeout time.Duration) {
}
func (s *stubLink) SetPacketDelivered(packet any, callback func(any)) {}
func (s *stubLink) HandleInbound(pkt *packet.Packet) error            { return nil }
func (s *stubLink) ValidateLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	return nil
}
func (s *stubLink) LinkedNetworkInterface() common.NetworkInterface { return nil }

func TestCanAcceptIncomingLinkLimit(t *testing.T) {
	tr := &Transport{links: make(map[hash16]LinkInterface)}
	if !tr.CanAcceptIncomingLink() {
		t.Fatal("empty transport should accept")
	}
	for i := range MaxRegisteredLinks {
		id := []byte{byte(i >> 8), byte(i)}
		for len(id) < 16 {
			id = append(id, 0)
		}
		tr.RegisterLink(id, &stubLink{})
	}
	if tr.LinkCount() != MaxRegisteredLinks {
		t.Fatalf("link count %d want %d", tr.LinkCount(), MaxRegisteredLinks)
	}
	if tr.CanAcceptIncomingLink() {
		t.Fatal("should reject at MaxRegisteredLinks")
	}
}

func TestIncomingLinkReservationLimit(t *testing.T) {
	tr := &Transport{links: make(map[hash16]LinkInterface)}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	n := MaxRegisteredLinks * 4
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			if tr.BeginIncomingHandshake() {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	got := int(accepted.Load())
	if got != MaxRegisteredLinks {
		t.Fatalf("accepted %d reservations, want %d", got, MaxRegisteredLinks)
	}
	if tr.CanAcceptIncomingLink() {
		t.Fatal("open reservations must count toward the incoming link limit")
	}
	for range got {
		tr.EndIncomingHandshake()
	}
	if !tr.CanAcceptIncomingLink() {
		t.Fatal("released reservations should accept again")
	}
}
