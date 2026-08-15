// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
	"bytes"
	"encoding/hex"
	"math"
	"testing"

	"quad4/reticulum-go/pkg/transport"
)

func TestOracleChannelConstantsMatchPythonRNS(t *testing.T) {
	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"WINDOW", float64(WindowInitial), 2},
		{"WINDOW_MIN", float64(WindowMin), 2},
		{"WINDOW_MIN_SLOW", float64(WindowMinSlow), 2},
		{"WINDOW_MIN_MEDIUM", float64(WindowMinMedium), 5},
		{"WINDOW_MIN_FAST", float64(WindowMinFast), 16},
		{"WINDOW_MAX_SLOW", float64(WindowMaxSlow), 5},
		{"WINDOW_MAX_MEDIUM", float64(WindowMaxMedium), 12},
		{"WINDOW_MAX_FAST", float64(WindowMaxFast), 48},
		{"WINDOW_FLEXIBILITY", float64(WindowFlexibility), 4},
		{"FAST_RATE_THRESHOLD", float64(FastRateThreshold), 10},
		{"RTT_FAST", RTTFast, 0.18},
		{"RTT_MEDIUM", RTTMedium, 0.75},
		{"RTT_SLOW", RTTSlow, 1.45},
		{"SEQ_MAX", float64(SeqMax), 65535},
		{"SEQ_MODULUS", float64(SeqModulus), 65536},
		{"HEADER", float64(ChannelHeaderSize), 6},
		{"MAX_TRIES", float64(DefaultMaxTries), 5},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("%s=%v want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestOracleChannelEnvelopePackMatchesPythonStruct(t *testing.T) {
	cases := []struct {
		msgtype uint16
		seq     uint16
		body    []byte
		rawHex  string
	}{
		{1, 0, []byte("test"), "00010000000474657374"},
		{2, 0xffff, nil, "0002ffff0000"},
		{171, 7, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, "00ab00070010000102030405060708090a0b0c0d0e0f"},
	}
	for _, tc := range cases {
		raw, err := packEnvelope(tc.msgtype, tc.seq, tc.body)
		if err != nil {
			t.Fatal(err)
		}
		want, err := hex.DecodeString(tc.rawHex)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw, want) {
			t.Fatalf("envelope %x want %x", raw, want)
		}
	}
}

func TestOracleChannelTimeoutMatchesPythonFormula(t *testing.T) {
	cases := []struct {
		tries int
		rtt   float64
		tx    int
		want  float64
	}{
		{1, 0.1, 1, 0.625},
		{2, 0.1, 2, 1.3125},
		{1, 0.01, 0, 0.0375},
		{3, 0.75, 5, 27.421875},
	}
	for _, tc := range cases {
		got := packetTimeoutSeconds(tc.tries, tc.rtt, tc.tx)
		if math.Abs(got-tc.want) > 1e-12 {
			t.Fatalf("tries=%d rtt=%v tx=%d timeout=%v want %v", tc.tries, tc.rtt, tc.tx, got, tc.want)
		}
	}
}

func TestOracleChannelSlowRTTUsesWindowOne(t *testing.T) {
	slow := NewChannel(&mockLink{rtt: 1.46, status: transport.StatusActive})
	if slow.Window() != 1 || slow.WindowMax() != 1 {
		t.Fatalf("slow RTT window=%d max=%d want 1", slow.Window(), slow.WindowMax())
	}
	normal := NewChannel(&mockLink{rtt: 0.2, status: transport.StatusActive})
	if normal.Window() != WindowInitial || normal.WindowMax() != WindowMaxSlow {
		t.Fatalf("normal window=%d max=%d", normal.Window(), normal.WindowMax())
	}
}

func TestOracleChannelWindowGrowsToMediumThenFast(t *testing.T) {
	c := NewChannel(&mockLink{rtt: 0.2, status: transport.StatusActive})
	c.mutex.Lock()
	for range FastRateThreshold {
		c.adjustWindowOnDeliveredLocked(0.5)
	}
	if c.windowMax != WindowMaxMedium || c.windowMin != WindowMinMedium {
		c.mutex.Unlock()
		t.Fatalf("after medium rounds max=%d min=%d", c.windowMax, c.windowMin)
	}
	for range FastRateThreshold {
		c.adjustWindowOnDeliveredLocked(0.1)
	}
	if c.windowMax != WindowMaxFast || c.windowMin != WindowMinFast {
		c.mutex.Unlock()
		t.Fatalf("after fast rounds max=%d min=%d", c.windowMax, c.windowMin)
	}
	c.mutex.Unlock()
}
