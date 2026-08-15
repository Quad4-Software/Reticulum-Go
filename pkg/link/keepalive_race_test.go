// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"sync"
	"testing"
	"time"
)

func TestRaceKeepaliveClocks(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.linkID = make([]byte, 16)
	l.status.Store(int32(StatusActive))
	l.keepalive = time.Hour
	l.staleTime = 2 * time.Hour
	l.establishedAt = time.Now()
	l.lastInboundNs.Store(time.Now().UnixNano())
	l.lastOutboundNs.Store(time.Now().UnixNano())
	l.startWatchdog()

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				l.recordOutboundData()
				l.recordKeepaliveOutbound()
				l.recordInbound(true)
				_ = initiatorShouldSendKeepalive(
					time.Now(),
					nsToTime(l.lastInboundNs.Load()),
					nsToTime(l.lastOutboundNs.Load()),
					nsToTime(l.lastKeepaliveNs.Load()),
					l.keepalive,
					true,
				)
			}
		})
	}
	wg.Wait()
	if l.lastOutboundNs.Load() == 0 || l.lastKeepaliveNs.Load() == 0 {
		t.Fatal("race run left clocks unset")
	}
}
