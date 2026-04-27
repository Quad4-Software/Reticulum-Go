// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package channel

import (
	"fmt"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

type scaleMockLink struct {
	status byte
}

func (m *scaleMockLink) GetStatus() byte                                       { return m.status }
func (m *scaleMockLink) GetRTT() float64                                       { return 0.1 }
func (m *scaleMockLink) RTT() float64                                          { return 0.1 }
func (m *scaleMockLink) GetLinkID() []byte                                     { return []byte("mocklink") }
func (m *scaleMockLink) Send(data []byte) any                                  { return "packet" }
func (m *scaleMockLink) Resend(p any) error                                    { return nil }
func (m *scaleMockLink) SetPacketTimeout(p any, cb func(any), t time.Duration) {}
func (m *scaleMockLink) SetPacketDelivered(p any, cb func(any))                {}
func (m *scaleMockLink) HandleInbound(pkt *packet.Packet) error                { return nil }
func (m *scaleMockLink) ValidateLinkProof(pkt *packet.Packet, iface common.NetworkInterface) error {
	return nil
}
func (m *scaleMockLink) LinkedNetworkInterface() common.NetworkInterface { return nil }

func BenchmarkChannelScale(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Handlers-%d", size), func(b *testing.B) {
			link := &scaleMockLink{status: transport.StatusActive}
			ch := NewChannel(link)

			for range size {
				ch.AddMessageHandler(func(m MessageBase) bool {
					return false // continue to next handler
				})
			}

			msg := &GenericMessage{Type: 1, Data: []byte("benchmark")}
			data := make([]byte, ChannelHeaderSize+len(msg.Data))
			// Manual pack for speed
			data[0], data[1] = 0, 1
			data[4], data[5] = 0, byte(len(msg.Data))
			copy(data[ChannelHeaderSize:], msg.Data)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = ch.HandleInbound(data)
			}
		})
	}
}

func BenchmarkChannelSendScale(b *testing.B) {
	link := &scaleMockLink{status: transport.StatusActive}
	ch := NewChannel(link)
	msg := &GenericMessage{Type: 1, Data: []byte("benchmark")}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ch.Send(msg)
		if i%100 == 0 {
			// Clear txRing to avoid infinite growth during benchmark
			ch.mutex.Lock()
			ch.txRing = nil
			ch.mutex.Unlock()
		}
	}
}
