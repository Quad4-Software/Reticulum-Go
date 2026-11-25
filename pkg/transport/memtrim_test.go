// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
)

func memTrimTestTransport(t *testing.T) *Transport {
	t.Helper()
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	t.Cleanup(func() { _ = tr.Close() })
	return tr
}

func setMemTrimState(last time.Time, baseline int64) {
	memTrimState.Lock()
	memTrimState.last = last
	memTrimState.baselineKiB = baseline
	memTrimState.Unlock()
}

func memTrimSnapshot() (time.Time, int64) {
	memTrimState.Lock()
	defer memTrimState.Unlock()
	return memTrimState.last, memTrimState.baselineKiB
}

func TestMemoryTrimStartupDeferralAndFirstBaseline(t *testing.T) {
	rss := procRSSKiB()
	if rss < 0 {
		t.Skip("RSS introspection unavailable")
	}
	tr := memTrimTestTransport(t)
	start := time.Now()
	setMemTrimState(start, -1)

	tr.maybeTrimMemory()
	last, baseline := memTrimSnapshot()
	if !last.Equal(start) || baseline != -1 {
		t.Fatalf("trim ran before hard floor: last=%v baseline=%d", last, baseline)
	}

	setMemTrimState(time.Now().Add(-memoryTrimFloor-time.Second), -1)
	tr.maybeTrimMemory()
	last, baseline = memTrimSnapshot()
	if baseline <= 0 {
		t.Fatalf("baseline = %d, want positive RSS after first due trim", baseline)
	}
	if time.Since(last) > time.Minute {
		t.Fatalf("last trim was not updated: %v", last)
	}
}

func TestMemoryTrimRSSGate(t *testing.T) {
	rss := procRSSKiB()
	if rss < 0 {
		t.Skip("RSS introspection unavailable")
	}
	tr := memTrimTestTransport(t)
	oldLast := time.Now().Add(-memoryTrimFloor - time.Second)

	setMemTrimState(oldLast, rss+memoryTrimGateKiB+1024)
	tr.maybeTrimMemory()
	last, _ := memTrimSnapshot()
	if !last.Equal(oldLast) {
		t.Fatalf("trim ran without RSS surplus: last %v want %v", last, oldLast)
	}

	setMemTrimState(oldLast, 1)
	tr.maybeTrimMemory()
	last, _ = memTrimSnapshot()
	if last.Equal(oldLast) {
		t.Fatal("trim did not run after RSS surplus reached the gate")
	}
}
