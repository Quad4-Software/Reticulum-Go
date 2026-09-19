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
	l.channelMutex.Lock()
	defer l.channelMutex.Unlock()

	if l.channel == nil {
		l.channel = channel.NewChannel(l)
	}
	return l.channel
}
func (l *Link) handleChannelPacket(pkt *packet.Packet) error {
	if !l.IsActive() {
		if l.status.Load() == int32(StatusHandshake) && l.sessionKey != nil {
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
func (l *Link) flushEarlyChannel() {
	l.earlyChannelMu.Lock()
	queued := l.earlyChannel
	l.earlyChannel = nil
	l.earlyChannelMu.Unlock()
	for _, pkt := range queued {
		if err := l.handleChannelPacket(pkt); err != nil {
			debug.Log(debug.DebugWarning, "Failed to flush early channel packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
		}
	}
}
