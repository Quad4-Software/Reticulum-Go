// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"fmt"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
)

type MockInterface struct {
	common.BaseInterface
	sentData  [][]byte
	dropRate  float64 // 0.0 to 1.0
	onReceive func([]byte)
}

func (m *MockInterface) Send(data []byte, destination string) error {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	m.sentData = append(m.sentData, data)
	return nil
}

func (m *MockInterface) Receive(data []byte) {
	if m.onReceive != nil {
		m.onReceive(data)
	}
}

func TestTransportNetworkSimulation(t *testing.T) {
	cfg := &common.ReticulumConfig{}
	tr := NewTransport(cfg)
	defer tr.Close()

	iface1 := &MockInterface{BaseInterface: common.NewBaseInterface("iface1", common.IFTypeUDP, true)}
	iface1.Enable()

	iface2 := &MockInterface{BaseInterface: common.NewBaseInterface("iface2", common.IFTypeUDP, true)}
	iface2.Enable()

	tr.RegisterInterface(iface1.GetName(), iface1)
	tr.RegisterInterface(iface2.GetName(), iface2)

	// Simulate receiving an announce on iface1
	// [header][hops][dest_hash(16)][payload...]
	announcePacket := make([]byte, 100)
	announcePacket[0] = PacketTypeAnnounce
	announcePacket[1] = 0 // 0 hops
	copy(announcePacket[2:18], []byte("destination_hash"))

	// Mock the handler to avoid complex identity logic in this basic test
	tr.HandlePacket(announcePacket, iface1)

	// In a real scenario, it would be rebroadcast to iface2
	// But HandlePacket runs in a goroutine, so we'd need to wait or use a better mock
	fmt.Println("Network simulation test initialized")
}
