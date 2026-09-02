// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/buffer"
	"quad4/reticulum-go/pkg/channel"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

func (n *Node) handleInterface(iface common.NetworkInterface) {
	debug.Log(debug.DebugInfo, "Setting up interface", "name", iface.GetName())
	ch := channel.NewChannel(&transportWrapper{n.transport})
	rw := buffer.CreateBidirectionalBuffer(
		1,
		2,
		ch,
		func(size int) {
			data := make([]byte, size)
			iface.ProcessIncoming(data)
			if len(data) > 0 {
				n.transport.HandlePacket(data, iface)
			}
		},
	)
	n.wiringMu.Lock()
	n.channels[iface.GetName()] = ch
	n.buffers[iface.GetName()] = &buffer.Buffer{ReadWriter: rw}
	n.wiringMu.Unlock()
}

func (n *Node) unregisterInterfaceBuffers(name string) {
	n.wiringMu.Lock()
	delete(n.channels, name)
	delete(n.buffers, name)
	n.wiringMu.Unlock()
}

type transportWrapper struct {
	*transport.Transport
}

func (tw *transportWrapper) GetRTT() float64 { return 0.1 }
func (tw *transportWrapper) RTT() float64    { return tw.GetRTT() }
func (tw *transportWrapper) GetStatus() byte { return transport.StatusActive }

func (tw *transportWrapper) Send(data []byte) any {
	p := &packet.Packet{
		PacketType: packet.PacketTypeData,
		Hops:       0,
		Data:       data,
		HeaderType: packet.HeaderType1,
	}
	if err := tw.Transport.SendPacket(p); err != nil {
		return nil
	}
	return p
}

func (tw *transportWrapper) Resend(p any) error {
	if pkt, ok := p.(*packet.Packet); ok {
		return tw.Transport.SendPacket(pkt)
	}
	return fmt.Errorf("invalid packet type")
}

func (tw *transportWrapper) SetPacketTimeout(packet any, callback func(any), timeout time.Duration) {
	time.AfterFunc(timeout, func() { callback(packet) })
}

func (tw *transportWrapper) SetPacketDelivered(packet any, callback func(any)) {
	callback(packet)
}

func (tw *transportWrapper) GetLinkID() []byte { return nil }

func (tw *transportWrapper) HandleInbound(pkt *packet.Packet) error { return nil }

func (tw *transportWrapper) ValidateLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	return nil
}

func (tw *transportWrapper) LinkedNetworkInterface() common.NetworkInterface { return nil }
