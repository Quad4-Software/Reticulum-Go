// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestReceivedAnnounceFrequency(t *testing.T) {
	bi := NewBaseInterface("freq", common.IFTypeUDP, true)
	if bi.IncomingAnnounceFrequency() != 0 {
		t.Fatalf("expected zero frequency before samples, got %v", bi.IncomingAnnounceFrequency())
	}
	for range 4 {
		bi.ReceivedAnnounce()
		time.Sleep(5 * time.Millisecond)
	}
	freq := bi.IncomingAnnounceFrequency()
	if freq <= 0 {
		t.Fatalf("expected positive announce frequency after samples, got %v", freq)
	}
}

func TestSampleTrafficSpeeds(t *testing.T) {
	bi := NewBaseInterface("speed", common.IFTypeUDP, true)
	bi.SampleTraffic()
	if bi.GetRxSpeed() != 0 || bi.GetTxSpeed() != 0 {
		t.Fatalf("expected zero speeds on first sample, got rx=%v tx=%v", bi.GetRxSpeed(), bi.GetTxSpeed())
	}
	bi.Mutex.Lock()
	bi.RxBytes += 1000
	bi.TxBytes += 2000
	bi.Mutex.Unlock()
	time.Sleep(20 * time.Millisecond)
	bi.SampleTraffic()
	if bi.GetRxSpeed() <= 0 {
		t.Fatalf("expected non-zero RX speed after byte increase, got %v", bi.GetRxSpeed())
	}
	if bi.GetTxSpeed() <= 0 {
		t.Fatalf("expected non-zero TX speed after byte increase, got %v", bi.GetTxSpeed())
	}
}

func TestGetBandwidthAvailable_UsesSampledTX(t *testing.T) {
	bi := NewBaseInterface("bw", common.IFTypeTCP, true)
	bi.Bitrate = BitrateGuess
	bi.lastTx = time.Now()
	bi.TxBytes = 1 << 20 // lifetime bytes must not alone close the gate
	if !bi.GetBandwidthAvailable() {
		t.Fatal("expected available without a TX sample")
	}
	bi.currentTXS = float64(bi.Bitrate) * PropagationRate * 2
	if bi.GetBandwidthAvailable() {
		t.Fatal("expected unavailable when sampled TX exceeds announce cap")
	}
	bi.currentTXS = float64(bi.Bitrate) * PropagationRate * 0.1
	if !bi.GetBandwidthAvailable() {
		t.Fatal("expected available when sampled TX is under announce cap")
	}
}
