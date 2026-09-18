// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"sync/atomic"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

// panicLinkDest panics inside the inbound link request handler to prove a
// hostile-input panic on the dispatch path cannot kill the process.
type panicLinkDest struct {
	called *atomic.Int32
}

func (d *panicLinkDest) HandleIncomingLinkRequest(pkt any, transport any, networkIface common.NetworkInterface) error {
	d.called.Add(1)
	panic("synthetic panic")
}

// countLinkDest records invocations without panicking.
type countLinkDest struct {
	called *atomic.Int32
}

func (d *countLinkDest) HandleIncomingLinkRequest(pkt any, transport any, networkIface common.NetworkInterface) error {
	d.called.Add(1)
	return nil
}

// buildShortLinkRequest builds a HeaderType1 LINKREQUEST whose payload after
// the context byte is shorter than 8 bytes.
func buildShortLinkRequest(destHash, linkID []byte) []byte {
	flags := byte(0)
	flags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationBroadcast << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationSingle << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeLinkReq & packet.HeaderMaskPacketType

	out := make([]byte, 0, 2+16+1+len(linkID))
	out = append(out, flags, 0)
	out = append(out, destHash...)
	out = append(out, packet.ContextNone)
	out = append(out, linkID...)
	return out
}

func TestRunPacketJobSurvivesHandlerPanic(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	iface := newOnlineTestIface("panic-iface")
	destHash := bytes.Repeat([]byte{0x33}, 16)
	var calls atomic.Int32
	tr.RegisterDestination(destHash, &panicLinkDest{called: &calls})

	raw, _ := buildHT1LinkRequest(t, destHash)
	pc := getPacketCopy(len(raw))
	copy(pc.buf, raw)

	// Before the recovery boundary this panic unwound the packet worker
	// goroutine and killed the process.
	tr.runPacketJob(packetJob{
		pc:         pc,
		iface:      iface,
		packetType: PacketTypeLink,
		destType:   DestTypeSingle,
		headerType: packet.HeaderType1,
	})
	if calls.Load() != 1 {
		t.Fatal("panicking handler was not invoked")
	}
}

func TestLinkRequestShortIDLogsSafely(t *testing.T) {
	debug.SetDebugLevel(debug.DebugVerbose)
	defer debug.SetDebugLevel(0)

	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	iface := newOnlineTestIface("shortid-iface")
	destHash := bytes.Repeat([]byte{0x55}, 16)
	var calls atomic.Int32
	tr.RegisterDestination(destHash, &countLinkDest{called: &calls})

	// A 3-byte link ID panicked on linkID[:8] under verbose logging.
	raw := buildShortLinkRequest(destHash, []byte{0x01, 0x02, 0x03})
	pc := getPacketCopy(len(raw))
	copy(pc.buf, raw)

	tr.runPacketJob(packetJob{
		pc:         pc,
		iface:      iface,
		packetType: PacketTypeLink,
		destType:   DestTypeSingle,
		headerType: packet.HeaderType1,
	})
	if calls.Load() != 1 {
		t.Fatal("handler was not invoked")
	}
}
