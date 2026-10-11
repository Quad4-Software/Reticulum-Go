// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"fmt"

	"github.com/Quad4-Software/Reticulum-Go/pkg/channel"
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

func (l *Link) GetChannel() *channel.Channel {
	l.channelMutex.RLock()
	if l.channel != nil {
		defer l.channelMutex.RUnlock()
		return l.channel
	}
	l.channelMutex.RUnlock()

	l.channelMutex.Lock()
	defer l.channelMutex.Unlock()
	if l.channel == nil {
		l.channel = channel.NewChannel(l)
	}
	return l.channel
}
func (l *Link) handleChannelPacket(pkt *packet.Packet) error {
	// The initiator's established callback installs channel handlers on its
	// own goroutine. Hold packets while the gate is up so a packet processed
	// between link activation and handler registration is not dropped by
	// the channel and the session stalls.
	if l.channelGate.Load() {
		if l.status.Load() == int32(StatusClosed) {
			return common.ErrLinkNotActive
		}
		l.earlyChannelMu.Lock()
		if l.channelGate.Load() {
			if len(l.earlyChannel) < maxEarlyChannelPackets {
				l.earlyChannel = append(l.earlyChannel, pkt)
			}
			l.earlyChannelMu.Unlock()
			return nil
		}
		l.earlyChannelMu.Unlock()
	}
	if !l.IsActive() {
		if l.status.Load() == int32(StatusHandshake) && l.hasSessionKeys() {
			l.queueEarlyChannel(pkt)
			return nil
		}
		return common.ErrLinkNotActive
	}

	plaintext, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
	}

	err = l.GetChannel().HandleInbound(plaintext)
	// Channel reliability depends on link proofs so the sender can clear its
	// TX ring only after the peer has processed the envelope.
	if proveErr := l.ProvePacket(pkt); proveErr != nil {
		debug.Log(debug.DebugWarning, "Failed to prove channel packet", "error", proveErr)
	} else {
		debug.Log(debug.DebugVerbose, "Proved channel packet", "link_id", fmt.Sprintf("%x", l.linkID))
	}
	return err
}

const maxEarlyChannelPackets = 32

func (l *Link) queueEarlyChannel(pkt *packet.Packet) {
	l.earlyChannelMu.Lock()
	defer l.earlyChannelMu.Unlock()
	if len(l.earlyChannel) >= maxEarlyChannelPackets {
		debug.Log(debug.DebugWarning, "Dropping early channel packet, queue full", "link_id", fmt.Sprintf("%x", l.linkID))
		return
	}
	l.earlyChannel = append(l.earlyChannel, pkt)
	debug.Log(debug.DebugVerbose, "Queued early channel packet until link active", "link_id", fmt.Sprintf("%x", l.linkID), "queued", len(l.earlyChannel))
}

// markChannelReady releases the channel gate and requeues packets that
// arrived while channel handlers were not yet installed at the head of
// the ordered inbound queue, so they are still processed before anything
// that arrived after them.
func (l *Link) markChannelReady() {
	// Release the gate under earlyChannelMu so a packet racing the release
	// either lands in earlyChannel before this grab or sees gate=false.
	l.earlyChannelMu.Lock()
	l.channelGate.Store(false)
	queued := l.earlyChannel
	l.earlyChannel = nil
	l.earlyChannelMu.Unlock()
	if len(queued) == 0 {
		return
	}
	l.inboundMu.Lock()
	l.inboundQ = append(queued, l.inboundQ...)
	spawn := !l.inboundBusy
	l.inboundBusy = true
	l.inboundMu.Unlock()
	if spawn {
		go l.drainInbound()
	}
}
