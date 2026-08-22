// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func newOnlineTestIface(name string) *common.BaseInterface {
	iface := common.NewBaseInterfacePtr(name, common.IFTypeUDP, true)
	iface.Online = true
	return iface
}

func saturateInboundDataQueue(t *testing.T, tr *Transport) {
	t.Helper()
	if tr == nil || tr.inboundQueues == nil {
		t.Fatal("no inbound queues")
	}
	size := tr.inboundQueueSizes[TCData]
	if size <= 0 {
		size = defaultInboundDataQueueLen
	}
	for i := 0; i < size; i++ {
		pc := getPacketCopy(2)
		pc.buf[0] = 0x00
		if !tr.inboundQueues.put(TCData, packetJob{pc: pc, packetType: 0}) {
			t.Fatalf("fill inbound queue at %d/%d", i, size)
		}
	}
}

func minimalDataPacket() []byte {
	tid := bytes.Repeat([]byte{0x01}, 16)
	dest := bytes.Repeat([]byte{0x02}, 16)
	return buildHT2Packet(tid, dest, 0, []byte{0xab})
}

func waitInboundDrain(t *testing.T, tr *Transport, extra time.Duration) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		total, _, _ := tr.inboundQueueSnapshot()
		if total == 0 {
			time.Sleep(extra)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("inbound queue did not drain")
}
