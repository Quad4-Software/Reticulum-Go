// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"os"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/protect"
)

// TestLinkSpeedSmoke is an RNS Speedtest-style loopback liveness floor.
// It skips under -short. Set RUN_SPEEDTEST=0 to skip explicitly.
func TestLinkSpeedSmoke(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	if os.Getenv("RUN_SPEEDTEST") == "0" {
		t.Skip("RUN_SPEEDTEST=0")
	}
	protect.SetDefault(protect.New(protect.Options{Mode: protect.ModeOff}))
	t.Cleanup(func() { protect.SetDefault(nil) })

	prev := debug.GetDebugLevel()
	debug.SetDebugLevel(debug.DebugCritical)
	t.Cleanup(func() { debug.SetDebugLevel(prev) })

	res, err := RunLoopbackSpeedtest(SpeedtestOptions{
		DataCap:        512 << 10, // 512 KiB keeps CI fast
		EnforceFloor:   true,
		MinBytesPerSec: DefaultSpeedtestMinBytesPerSec,
		Timeout:        20 * time.Second,
	})
	if err != nil {
		t.Fatalf("loopback speedtest: %v (sent=%d recv=%d mdu=%d dur=%v)",
			err, res.BytesSent, res.BytesRecv, res.MDU, res.Duration)
	}
	if res.BytesRecv < 512<<10 {
		t.Fatalf("recv=%d want >= %d", res.BytesRecv, 512<<10)
	}
	t.Logf("loopback speedtest: %.2f MB/s mdu=%d sent=%d recv=%d in %v",
		res.BytesPerSec/1e6, res.MDU, res.BytesSent, res.BytesRecv, res.Duration)
}
