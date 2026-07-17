package transport

import (
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
