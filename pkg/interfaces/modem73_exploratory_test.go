// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"testing"
)

func TestModem73MTUExploratoryInvariants(t *testing.T) {
	floor := 500
	for overhead := 1; overhead <= 40; overhead++ {
		prev := modem73ComputeMTU(0, overhead, floor)
		for ps := 0; ps <= 2000; ps += 17 {
			mtu := modem73ComputeMTU(ps, overhead, floor)
			if mtu < floor {
				t.Fatalf("mtu %d below floor", mtu)
			}
			if ps >= overhead && mtu < prev && ps-overhead >= floor {
				t.Fatalf("mtu decreased: ps=%d oh=%d mtu=%d prev=%d", ps, overhead, mtu, prev)
			}
			if ps > overhead {
				want := max(ps-overhead, floor)
				if mtu != want {
					t.Fatalf("ps=%d oh=%d mtu=%d want=%d", ps, overhead, mtu, want)
				}
			}
			prev = mtu
			frag := modem73NeedsFragmentation(ps, overhead, floor)
			if frag != ((ps - overhead) < floor) {
				t.Fatalf("frag mismatch ps=%d", ps)
			}
		}
	}
}

func TestModem73BitrateExploratoryInvariants(t *testing.T) {
	cfg := map[string]any{
		"modem_type":    float64(modem73TypeRobust),
		"robust_mode":   float64(0),
		"csma_enabled":  true,
		"csma_quiet_ms": float64(500),
		"csma_cw":       float64(8),
		"slot_time_ms":  float64(500),
		"csma_burst":    float64(1),
	}
	bps, ok := modem73TimeoutBitrate(cfg, true, 0.35)
	if !ok || bps < 8 {
		t.Fatalf("bps=%d ok=%v", bps, ok)
	}
	prof, ok := modem73PhyProfileFromCfg(cfg)
	if !ok || prof.airtime <= 0 || prof.phyBPS <= 0 {
		t.Fatalf("profile=%+v", prof)
	}
	oh := modem73CSMAPerFrameOverhead(cfg, prof.airtime)
	if oh < 0 {
		t.Fatalf("overhead=%v", oh)
	}
	duty := prof.airtime / (prof.airtime + oh)
	if duty <= 0 || duty > 1 {
		t.Fatalf("duty=%v", duty)
	}
}

func TestModem73ShortFrameExploratory(t *testing.T) {
	cfg := map[string]any{
		"modem_type":  float64(modem73TypeOFDM),
		"modulation":  "QPSK",
		"code_rate":   "1/2",
		"short_frame": false,
	}
	mode, ok := modem73ShortOperMode(cfg)
	if !ok || mode < 0 {
		t.Fatalf("mode=%d ok=%v", mode, ok)
	}
	cfg["short_frame"] = true
	if _, ok := modem73ShortOperMode(cfg); ok {
		t.Fatal("short_frame already on should disable override")
	}
}

func TestModem73PathTimeoutExploratory(t *testing.T) {
	sec := modem73PathRequestTimeoutSec(400, 500)
	if sec < 10 {
		t.Fatalf("sec=%d", sec)
	}
	if modem73PathRequestTimeoutSec(0, 500) != 0 {
		t.Fatal("zero bitrate")
	}
}

func TestModem73ControlFuzzSeeds(t *testing.T) {
	// sanity seeds used by fuzz corpus
	for _, body := range [][]byte{nil, []byte("{}"), []byte(`{"cmd":"get_config"}`)} {
		frame, err := modem73EncodeControl(map[string]any{"raw": string(body)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := readModem73Control(bytes.NewReader(frame)); err != nil {
			t.Fatal(err)
		}
	}
}
