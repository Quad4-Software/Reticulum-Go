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

	opt := SpeedtestOptions{
		DataCap:        512 << 10, // 512 KiB keeps CI fast
		EnforceFloor:   true,
		MinBytesPerSec: DefaultSpeedtestMinBytesPerSec,
		Timeout:        20 * time.Second,
		// Tiny pace avoids rare CI recv stalls when many packages share CPUs.
		SendPace: time.Microsecond,
	}
	var res SpeedtestResult
	var err error
	for attempt := 1; attempt <= 2; attempt++ {
		res, err = RunLoopbackSpeedtest(opt)
		if err == nil && res.BytesRecv >= opt.DataCap {
			break
		}
		if attempt == 1 {
			t.Logf("loopback speedtest attempt 1: %v (sent=%d recv=%d) retrying",
				err, res.BytesSent, res.BytesRecv)
		}
	}
	if err != nil {
		t.Fatalf("loopback speedtest: %v (sent=%d recv=%d mdu=%d dur=%v)",
			err, res.BytesSent, res.BytesRecv, res.MDU, res.Duration)
	}
	if res.BytesRecv < opt.DataCap {
		t.Fatalf("recv=%d want >= %d", res.BytesRecv, opt.DataCap)
	}
	t.Logf("loopback speedtest: %.2f MB/s mdu=%d sent=%d recv=%d in %v",
		res.BytesPerSec/1e6, res.MDU, res.BytesSent, res.BytesRecv, res.Duration)
}
