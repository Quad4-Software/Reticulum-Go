// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"testing"
	"time"

	"quad4/pbt/pkg/pbt"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

func TestPBTLinkStatusLaws(t *testing.T) {
	op := pbt.IntRange(0, 5)
	prop := pbt.ForAll(
		"link status laws under random ops",
		op,
		func(kind int) bool {
			cfg := &common.ReticulumConfig{}
			tr := transport.NewTransport(cfg)
			defer tr.Close()
			id, err := identity.New()
			if err != nil {
				return false
			}
			dest, err := destination.New(id, destination.Out, destination.Single, "pbtlink", tr, "svc")
			if err != nil {
				return false
			}
			l := NewLink(dest, tr, nil, nil, nil)
			l.linkID = make([]byte, 16)
			_ = setSecBuf(&l.sessionKey, make([]byte, 32))
			_ = setSecBuf(&l.hmacKey, make([]byte, 32))

			switch kind {
			case 0:
				l.status.Store(int32(StatusPending))
				l.establishmentTimeout = 15 * time.Millisecond
				l.requestTime = time.Now().Add(-time.Second)
				l.startWatchdog()
				deadline := time.Now().Add(time.Second)
				for time.Now().Before(deadline) && l.GetStatus() != StatusClosed {
					time.Sleep(2 * time.Millisecond)
				}
			case 1:
				l.status.Store(int32(StatusActive))
				l.Teardown()
			case 2:
				l.status.Store(int32(StatusStale))
				pkt := &packet.Packet{
					PacketType:      packet.PacketTypeData,
					Context:         packet.ContextNone,
					DestinationType: DestTypeLink,
					DestinationHash: l.linkID,
					Data:            []byte{0x00},
				}
				_ = l.HandleInbound(pkt)
			case 3:
				l.status.Store(int32(StatusActive))
				if err := l.Reestablish(); err == nil {
					return false
				}
			case 4:
				l.status.Store(int32(StatusClosed))
				l.Teardown()
				if l.GetStatus() != StatusClosed {
					return false
				}
			default:
				l.status.Store(int32(StatusHandshake))
				l.establishmentTimeout = 15 * time.Millisecond
				l.requestTime = time.Now().Add(-time.Second)
				l.startWatchdog()
				deadline := time.Now().Add(time.Second)
				for time.Now().Before(deadline) && l.GetStatus() != StatusClosed {
					time.Sleep(2 * time.Millisecond)
				}
			}

			st := l.GetStatus()
			if st == StatusActive && (bufLen(l.sessionKey) == 0 || bufLen(l.hmacKey) == 0) {
				return false
			}
			if st == StatusHandshake && kind == 4 {
				return false
			}
			return true
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(40), pbt.WithSeed(51))
}
