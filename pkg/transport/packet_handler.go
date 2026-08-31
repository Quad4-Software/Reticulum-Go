// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"fmt"
	"runtime"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/profiler"
	"quad4/reticulum-go/pkg/protect"
)

type packetJob struct {
	pc         packetCopy
	iface      common.NetworkInterface
	packetType byte
	destType   byte
	headerType byte
	hold       <-chan struct{}
}

func (t *Transport) startPacketWorkers(n int) {
	if n < 1 {
		n = common.DefaultMaxPacketHandlers
	}
	if t.packetQ == nil {
		t.packetQ = make(chan packetJob, n)
	}
	t.handlerWG.Add(n)
	t.handlerLive.Add(int32(n))
	for range n {
		go t.packetWorker()
	}
}

func startupHandlerCount(maxN int) int {
	if maxN < 1 {
		maxN = common.DefaultMaxPacketHandlers
	}
	boot := max(runtime.GOMAXPROCS(0), 4)
	if maxN < boot {
		return maxN
	}
	return boot
}

func (t *Transport) ensurePacketWorkers() {
	t.handlerOnce.Do(func() {
		if t.handlerClosed.Load() {
			return
		}
		select {
		case <-t.done:
			return
		default:
			t.startPacketWorkers(startupHandlerCount(t.handlerN))
		}
	})
}

func (t *Transport) growOneHandler() bool {
	if t.handlerClosed.Load() {
		return false
	}
	t.growMu.Lock()
	defer t.growMu.Unlock()
	if t.handlerClosed.Load() {
		return false
	}
	select {
	case <-t.done:
		return false
	default:
	}
	if int(t.handlerLive.Load()) >= t.handlerN {
		return false
	}
	t.handlerWG.Add(1)
	t.handlerLive.Add(1)
	go t.packetWorker()
	return true
}

func (t *Transport) growHandlersToMax() {
	for t.growOneHandler() {
	}
}

func (t *Transport) packetWorker() {
	defer t.handlerWG.Done()
	for {
		select {
		case <-t.done:
			return
		default:
		}
		select {
		case <-t.done:
			return
		case job, ok := <-t.packetQ:
			if !ok {
				return
			}
			t.runPacketJob(job)
		}
	}
}

func (t *Transport) runPacketJob(job packetJob) {
	span := profiler.Start("Transport.runPacketJob")
	defer span.End()
	if job.hold != nil {
		select {
		case <-job.hold:
		case <-t.done:
		}
		return
	}
	defer putPacketCopy(job.pc)
	t.dispatchInboundPacket(job.pc.buf, job.iface, job.packetType, job.destType, job.headerType)
}

func (t *Transport) enqueuePacket(job packetJob, block bool) bool {
	t.ensurePacketWorkers()
	select {
	case <-t.done:
		putPacketCopy(job.pc)
		return true
	case t.packetQ <- job:
		return true
	default:
	}
	if t.growOneHandler() {
		select {
		case t.packetQ <- job:
			return true
		default:
		}
	}
	if !block {
		return false
	}
	select {
	case <-t.done:
		putPacketCopy(job.pc)
		return true
	default:
	}
	select {
	case <-t.done:
		putPacketCopy(job.pc)
		return true
	case t.packetQ <- job:
		return true
	}
}

func (t *Transport) occupyHandlerPoolForTest(hold <-chan struct{}) int {
	t.ensurePacketWorkers()
	t.growHandlersToMax()
	n := cap(t.packetQ)
	if n < 1 {
		return 0
	}
	filled := 0
	for range n * 2 {
		t.packetQ <- packetJob{hold: hold}
		filled++
	}
	return filled
}

func (t *Transport) dispatchInboundPacket(payload []byte, iface common.NetworkInterface, packetType, destType, headerType byte) {
	switch packetType {
	case PacketTypeAnnounce:
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Processing announce packet")
		}
		if err := t.handleAnnouncePacket(payload, iface); err != nil {
			debug.Log(debug.DebugWarning, "Announce handling failed", "error", err)
		}
	case PacketTypeLink:
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Processing link packet (type=0x02)", "packet_size", len(payload))
		}
		t.handleLinkPacket(payload, iface, PacketTypeLink)
	case packet.PacketTypeProof:
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Processing proof packet")
		}
		pkt := &packet.Packet{Raw: payload}
		if err := pkt.Unpack(); err != nil {
			if debug.Enabled(debug.DebugInfo) {
				debug.Log(debug.DebugWarning, "Failed to unpack proof packet", "error", err)
			}
			ifaceName := ""
			if iface != nil {
				ifaceName = iface.GetName()
			}
			health.Inc(ifaceName, health.KindUnpackFail)
			return
		}
		t.handleProofPacket(pkt, iface)
	case 0:
		if destType == DestTypeLink {
			if debug.Enabled(debug.DebugVerbose) {
				debug.Log(debug.DebugVerbose, "Processing link data packet (dest_type=3)", "packet_size", len(payload))
			}
			t.handleLinkPacket(payload, iface, 0)
		} else {
			if debug.Enabled(debug.DebugVerbose) {
				debug.Log(debug.DebugVerbose, "Processing data packet (type 0x00)", "packet_size", len(payload), "dest_type", destType, "header_type", headerType)
			}
			t.handleTransportPacket(payload, iface)
		}
	default:
		src := ""
		if iface != nil {
			src = iface.GetName()
		}
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Unknown packet type", "type", fmt.Sprintf("0x%02x", packetType), "source", src)
		}
	}
}

func (t *Transport) shedHandlerOverflow(iface common.NetworkInterface) {
	ifaceName := ""
	if iface != nil {
		ifaceName = iface.GetName()
	}
	protect.AdmitHandler(ifaceName)
	if protect.Default().Mode() == protect.ModeOff {
		health.Inc(ifaceName, health.KindDoSHandler)
	}
}
