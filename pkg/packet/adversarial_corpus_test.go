// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

type adversarialManifest struct {
	FormatVersion int               `json:"format_version"`
	Cases         []adversarialCase `json:"cases"`
}

type adversarialCase struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Expect         string `json:"expect"`
	ErrorContains  string `json:"error_contains"`
	PacketTypeName string `json:"packet_type_name"`
	RawHex         string `json:"raw_hex"`
}

func TestAdversarialCorpus(t *testing.T) {
	dir := filepath.Join("testdata", "adversarial")
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var man adversarialManifest
	if err := json.Unmarshal(raw, &man); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if man.FormatVersion < 1 {
		t.Fatalf("unsupported manifest format_version %d", man.FormatVersion)
	}
	if len(man.Cases) == 0 {
		t.Fatal("manifest has no cases")
	}

	var ann *announce.Announce
	for _, c := range man.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if c.RawHex == "" {
				t.Fatal("case missing raw_hex")
			}
			data, err := hex.DecodeString(c.RawHex)
			if err != nil {
				t.Fatalf("raw_hex: %v", err)
			}
			switch c.Kind {
			case "unpack":
				p := &Packet{Raw: append([]byte(nil), data...)}
				err := p.Unpack()
				assertErrorOutcome(t, c, err)
			case "decode":
				fr := DecodeFrame(data)
				if c.Expect == "ok" {
					if !fr.OK {
						t.Fatalf("DecodeFrame error: %s", fr.Error)
					}
					if c.PacketTypeName != "" && fr.PacketTypeName != c.PacketTypeName {
						t.Fatalf("PacketTypeName = %q, want %q", fr.PacketTypeName, c.PacketTypeName)
					}
					return
				}
				if fr.OK {
					t.Fatal("expected DecodeFrame failure")
				}
				if c.ErrorContains != "" && !strings.Contains(fr.Error, c.ErrorContains) {
					t.Fatalf("error %q does not contain %q", fr.Error, c.ErrorContains)
				}
			case "pcap":
				_, err := ReadPCAPUDPPayloads(bytes.NewReader(data))
				assertErrorOutcome(t, c, err)
			case "pcap_empty_udp":
				caps, err := ReadPCAPUDPPayloads(bytes.NewReader(data))
				if err != nil {
					t.Fatalf("ReadPCAPUDPPayloads: %v", err)
				}
				if len(caps) != 1 {
					t.Fatalf("caps = %d, want 1", len(caps))
				}
				if caps[0].Payload == nil {
					t.Fatal("empty UDP payload must be non-nil empty slice")
				}
				if len(caps[0].Payload) != 0 {
					t.Fatalf("payload len = %d, want 0", len(caps[0].Payload))
				}
			case "announce":
				if ann == nil {
					id, err := identity.New()
					if err != nil {
						t.Fatalf("identity.New: %v", err)
					}
					ann, err = announce.New(id, make([]byte, 16), "advapp", nil, false, &common.ReticulumConfig{})
					if err != nil {
						t.Fatalf("announce.New: %v", err)
					}
				}
				err := ann.HandleAnnounce(data)
				assertErrorOutcome(t, c, err)
			default:
				t.Fatalf("unknown kind %q", c.Kind)
			}
		})
	}
}

func assertErrorOutcome(t *testing.T, c adversarialCase, err error) {
	t.Helper()
	if c.Expect == "ok" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatal("expected error")
	}
	if c.ErrorContains != "" && !strings.Contains(err.Error(), c.ErrorContains) {
		t.Fatalf("error %q does not contain %q", err.Error(), c.ErrorContains)
	}
}
