// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"quad4/pbt/pkg/pbt"
	"quad4/reticulum-go/pkg/common"
)

func TestOracleBlockedIPsMatchesCount(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{
		Enabled:              true,
		Port:                 1,
		BlockFastFlapping:    true,
		BlockFastFlappingSet: true,
	}
	bi, err := NewBackboneInterface("oracle-block", cfg, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	bi.fastFlapGrace = 1
	bi.fastFlapThreshold = time.Minute

	ips := []string{"198.51.100.1", "198.51.100.2", "203.0.113.8"}
	for _, ip := range ips {
		bi.recordFastFlap(ip, time.Millisecond)
		bi.recordFastFlap(ip, time.Millisecond)
	}
	list := bi.BlockedIPs()
	if bi.BlockedIPCount() != len(list) {
		t.Fatalf("count=%d list=%d want equal", bi.BlockedIPCount(), len(list))
	}
	if len(list) != 3 {
		t.Fatalf("blocked=%v want 3", list)
	}
	for i := 1; i < len(list); i++ {
		if list[i-1] >= list[i] {
			t.Fatalf("BlockedIPs not sorted: %v", list)
		}
	}
}

func TestAdversarialBlockedIPInputs(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{Enabled: true, Port: 1, BlockFastFlappingSet: true, BlockFastFlapping: true}
	bi, err := NewBackboneInterface("adv-block", cfg, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	bi.fastFlapGrace = 0
	bi.recordFastFlap("", time.Millisecond)
	bi.recordFastFlap("198.51.100.7", 10*time.Minute)
	if bi.BlockedIPCount() != 0 {
		t.Fatal("empty IP and long-lived flaps must not block")
	}
	bi.blockFastFlapping = false
	bi.recordFastFlap("198.51.100.7", time.Millisecond)
	if bi.BlockedIPCount() != 0 || bi.BlockedIPs() != nil {
		t.Fatal("disabled flap block must export empty")
	}
}

func TestPBTBlockedIPCountListInvariant(t *testing.T) {
	hub := testBackboneHub(t)
	flaps := pbt.IntRange(0, 6)
	prop := pbt.ForAll(
		"BlockedIPCount equals len(BlockedIPs) for grace=0",
		flaps,
		func(n int) bool {
			cfg := &common.InterfaceConfig{Enabled: true, Port: 1, BlockFastFlappingSet: true, BlockFastFlapping: true}
			bi, err := NewBackboneInterface("pbt-block", cfg, hub, nil)
			if err != nil {
				return false
			}
			bi.fastFlapGrace = 0
			bi.fastFlapThreshold = time.Minute
			for i := range n {
				bi.recordFastFlap(netIPFixture(i), time.Millisecond)
			}
			list := bi.BlockedIPs()
			return bi.BlockedIPCount() == len(list) && len(list) == n
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(16), pbt.WithSeed(146))
}

func netIPFixture(i int) string {
	return fmt.Sprintf("198.51.100.%d", (i%200)+1)
}

func TestRaceBlockedIPExport(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{Enabled: true, Port: 1, BlockFastFlappingSet: true, BlockFastFlapping: true}
	bi, err := NewBackboneInterface("race-block", cfg, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	bi.fastFlapGrace = 0
	bi.fastFlapThreshold = time.Minute

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for i := range 100 {
				bi.recordFastFlap(netIPFixture(i%10), time.Millisecond)
				_ = bi.BlockedIPCount()
				_ = bi.BlockedIPs()
				_ = bi.isFastFlappingBlocked(netIPFixture(i % 10))
			}
		})
	}
	wg.Wait()
}
