// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"testing"
	"time"

	"quad4/pbt/pkg/pbt"
)

func TestPBTKeepaliveInitiatorLaws(t *testing.T) {
	offsetMS := pbt.IntRange(0, 120)
	prop := pbt.ForAll(
		"initiator send requires due keepalive and aged lastKeepalive",
		offsetMS,
		func(inboundAgeMS int) bool {
			ka := 40 * time.Millisecond
			now := time.Now()
			inbound := now.Add(-time.Duration(inboundAgeMS) * time.Millisecond)
			outbound := now
			lastKA := now.Add(-time.Minute)
			got := initiatorShouldSendKeepalive(now, inbound, outbound, lastKA, ka, true)
			need := keepaliveDue(now, inbound, outbound, ka)
			if !need && got {
				return false
			}
			if need && !now.After(lastKA.Add(ka)) && got {
				return false
			}
			if need && now.After(lastKA.Add(ka)) && !got {
				return false
			}
			return true
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(140))
}

func TestPBTKeepaliveResponderThrottle(t *testing.T) {
	offsetMS := pbt.IntRange(0, 200)
	prop := pbt.ForAll(
		"responder reply only when outbound aged past keepalive",
		offsetMS,
		func(outboundAgeMS int) bool {
			ka := 50 * time.Millisecond
			now := time.Now()
			outbound := now.Add(-time.Duration(outboundAgeMS) * time.Millisecond)
			got := responderShouldReplyKeepalive(now, outbound, ka, false)
			want := !now.Before(outbound.Add(ka))
			if got != want {
				return false
			}
			if responderShouldReplyKeepalive(now, outbound, ka, true) {
				return false
			}
			return true
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(141))
}

func TestPBTKeepaliveDueSymmetric(t *testing.T) {
	offsetMS := pbt.IntRange(0, 100)
	prop := pbt.ForAll(
		"keepaliveDue is true iff inbound or outbound is aged",
		offsetMS,
		func(ageMS int) bool {
			ka := 30 * time.Millisecond
			now := time.Now()
			aged := now.Add(-time.Duration(ageMS) * time.Millisecond)
			fresh := now
			a := keepaliveDue(now, aged, fresh, ka)
			b := keepaliveDue(now, fresh, aged, ka)
			c := keepaliveDue(now, fresh, fresh, ka)
			wantAged := ageMS > 30
			if a != wantAged || b != wantAged {
				return false
			}
			return !c
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(60), pbt.WithSeed(142))
}
