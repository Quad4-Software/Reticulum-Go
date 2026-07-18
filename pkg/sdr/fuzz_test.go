// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sdr

import "testing"

func FuzzSDRBurstModem(f *testing.F) {
	f.Add([]byte("hi"))
	f.Add([]byte{})
	f.Add(bytesRepeat(100, 0xA5))
	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > burstMaxPayload {
			payload = payload[:burstMaxPayload]
		}
		m := NewBurstModem()
		iq, err := m.Encode(payload)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := m.Decode(iq)
		if !ok || !bytesEqual(got, payload) {
			t.Fatalf("roundtrip failed len=%d", len(payload))
		}
		_, _ = m.Decode(iq[len(iq)/2:])
	})
}

func FuzzSDRDeviceConfig(f *testing.F) {
	f.Add(int64(433000000), 2000000, 20.0)
	f.Fuzz(func(t *testing.T, freq int64, rate int, gain float64) {
		_ = ClampFrequency(freq)
		_ = ClampSampleRate(rate)
		_ = ClampGain(gain)
	})
}

func bytesRepeat(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
