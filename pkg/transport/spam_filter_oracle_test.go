// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

type spamFilterFile struct {
	Amplification []spamFilterCase `json:"amplification"`
}

type spamFilterCase struct {
	Name           string  `json:"name"`
	Kind           string  `json:"kind"`
	Inject         int     `json:"inject"`
	MaxForward     int     `json:"max_forward"`
	ICBurstFreq    float64 `json:"ic_burst_freq"`
	ICBurstFreqNew float64 `json:"ic_burst_freq_new"`
	ICNewTime      int     `json:"ic_new_time"`
}

func loadSpamFilterVectors(t *testing.T) []spamFilterCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "spam_filter_vectors.json"))
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var file spamFilterFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(file.Amplification) == 0 {
		t.Fatal("no amplification vectors")
	}
	return file.Amplification
}

func spamFilterTransport(t *testing.T, inCfg *common.InterfaceConfig, gatewayIn bool) (*Transport, *trackingIface, *trackingIface) {
	t.Helper()
	cfg := &common.ReticulumConfig{
		EnableTransport: true,
		DoSProtection:   "off",
		Interfaces:      map[string]*common.InterfaceConfig{},
	}
	if inCfg != nil {
		cfg.Interfaces["in"] = inCfg
	}
	tr := NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	in := newTrackingIface("in")
	out := newTrackingIface("out")
	if gatewayIn {
		in.Mode = common.IFModeGateway
	}
	if err := tr.RegisterInterface("in", in); err != nil {
		t.Fatalf("register in: %v", err)
	}
	if err := tr.RegisterInterface("out", out); err != nil {
		t.Fatalf("register out: %v", err)
	}
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}
	return tr, in, out
}

func flushAnnounceForwards(tr *Transport) {
	for range 8 {
		tr.processDelayedAnnounceJobs()
	}
}

func prPayload(destHash, tag []byte) []byte {
	out := make([]byte, 0, len(destHash)+len(tag))
	out = append(out, destHash...)
	out = append(out, tag...)
	return out
}

func TestOracleSpamFilterNoAmplification(t *testing.T) {
	withFastAnnounceForward(t)
	for _, tc := range loadSpamFilterVectors(t) {
		t.Run(tc.Name, func(t *testing.T) {
			runSpamFilterCase(t, tc)
		})
	}
}

func runSpamFilterCase(t *testing.T, tc spamFilterCase) {
	t.Helper()
	if tc.Inject <= 0 {
		t.Fatal("inject must be positive")
	}

	var inCfg *common.InterfaceConfig
	if tc.Kind == "announce_unique_flood" {
		inCfg = &common.InterfaceConfig{
			IngressControlSet:  true,
			IngressControl:     true,
			ICBurstFreq:        tc.ICBurstFreq,
			ICBurstFreqNew:     tc.ICBurstFreqNew,
			ICNewTime:          tc.ICNewTime,
			ICBurstHold:        5,
			ICBurstPenalty:     5,
			ICMaxHeldAnnounces: 16,
		}
	}

	gateway := tc.Kind == "pr_dup" || tc.Kind == "pr_unique_flood"
	tr, in, out := spamFilterTransport(t, inCfg, gateway)

	switch tc.Kind {
	case "announce_dup":
		id, err := identity.New()
		if err != nil {
			t.Fatalf("identity: %v", err)
		}
		raw, _ := signedAnnounce(t, tr, id)
		for range tc.Inject {
			if err := tr.handleAnnouncePacket(raw, in); err != nil {
				t.Fatalf("announce: %v", err)
			}
		}
		flushAnnounceForwards(tr)
	case "announce_unique_flood":
		for range tc.Inject {
			id, err := identity.New()
			if err != nil {
				t.Fatalf("identity: %v", err)
			}
			raw, _ := signedAnnounce(t, tr, id)
			if err := tr.handleAnnouncePacket(raw, in); err != nil {
				t.Fatalf("announce: %v", err)
			}
		}
		flushAnnounceForwards(tr)
	case "announce_bad_sig":
		for range tc.Inject {
			id, err := identity.New()
			if err != nil {
				t.Fatalf("identity: %v", err)
			}
			raw, _ := signedAnnounce(t, tr, id)
			raw[len(raw)-1] ^= 0xff
			_ = tr.handleAnnouncePacket(raw, in)
		}
		flushAnnounceForwards(tr)
	case "announce_path_response":
		for range tc.Inject {
			id, err := identity.New()
			if err != nil {
				t.Fatalf("identity: %v", err)
			}
			raw, _ := signedAnnounceWithContext(t, tr, id, packet.ContextPathResponse)
			if err := tr.handleAnnouncePacket(raw, in); err != nil {
				t.Fatalf("path response: %v", err)
			}
		}
		flushAnnounceForwards(tr)
	case "pr_dup":
		dest := randomDestHash(1)
		tag := randomDestHash(2)
		payload := prPayload(dest, tag)
		for range tc.Inject {
			tr.handlePathRequest(payload, in)
		}
	case "pr_unique_flood":
		for i := range tc.Inject {
			dest := randomDestHash(i + 1)
			tag := randomDestHash(i + 1000)
			tr.handlePathRequest(prPayload(dest, tag), in)
		}
		tr.pendingDiscoveryPRMu.Lock()
		queued := len(tr.pendingDiscoveryPRs)
		tr.pendingDiscoveryPRMu.Unlock()
		if queued > maxQueuedDiscoveryPRs {
			t.Fatalf("discovery PR queue %d exceeds cap %d", queued, maxQueuedDiscoveryPRs)
		}
	case "pr_tagless":
		dest := randomDestHash(3)
		for range tc.Inject {
			tr.handlePathRequest(dest, in)
		}
	case "pr_full_mode_no_discover":
		for i := range tc.Inject {
			dest := randomDestHash(i + 1)
			tag := randomDestHash(i + 2000)
			tr.handlePathRequest(prPayload(dest, tag), in)
		}
	default:
		t.Fatalf("unknown kind %q", tc.Kind)
	}

	if got := sentCount(out); got > tc.MaxForward {
		t.Fatalf("forwarded %d packets, golden max_forward=%d (kind=%s inject=%d)", got, tc.MaxForward, tc.Kind, tc.Inject)
	}
}
