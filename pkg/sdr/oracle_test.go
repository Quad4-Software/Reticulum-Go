// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sdr

import (
	"bytes"
	"testing"
)

func TestBurstModemOracleInvariants(t *testing.T) {
	m := NewBurstModem()
	for n := 0; n <= 200; n += 7 {
		payload := bytes.Repeat([]byte{byte(n)}, n)
		iq, err := m.Encode(payload)
		if err != nil {
			t.Fatal(err)
		}
		if len(iq) == 0 {
			t.Fatal("empty iq")
		}
		got, ok := m.Decode(iq)
		if !ok || !bytes.Equal(got, payload) {
			t.Fatalf("n=%d ok=%v", n, ok)
		}
		// Leading noise must not panic and may fail CRC
		noisy := append([]Complex64{{I: 0.1, Q: 0.1}, {I: -0.1, Q: 0}}, iq...)
		_, _ = m.Decode(noisy)
	}
	if _, err := m.Encode(bytes.Repeat([]byte{1}, burstMaxPayload+1)); err == nil {
		t.Fatal("expected too large")
	}
}

func TestDeviceConfigOracle(t *testing.T) {
	for _, rate := range []int{-1, 0, 100, 250000, 2000000, 50000000} {
		c := ClampSampleRate(rate)
		if c < 250000 || c > 20000000 {
			t.Fatalf("rate=%d clamped=%d", rate, c)
		}
	}
	for _, g := range []float64{-10, 0, 30, 90} {
		c := ClampGain(g)
		if c < 0 || c > 60 {
			t.Fatalf("gain=%v clamped=%v", g, c)
		}
	}
}
