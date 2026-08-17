// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type packetClassFile struct {
	Vectors []packetClassVector `json:"vectors"`
}

type packetClassVector struct {
	Name   string `json:"name"`
	RawHex string `json:"raw_hex"`
	Class  string `json:"class"`
}

func parsePacketClassName(s string) PacketClass {
	switch s {
	case "shed_first":
		return ClassShedFirst
	case "prefer_keep":
		return ClassPreferKeep
	default:
		return ClassUnknown
	}
}

func TestOraclePacketClassWireVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "packet_class_vectors.json"))
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var file packetClassFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(file.Vectors) == 0 {
		t.Fatal("no packet class vectors")
	}
	for _, v := range file.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			pkt, err := hex.DecodeString(v.RawHex)
			if err != nil {
				t.Fatalf("hex: %v", err)
			}
			got := PeekPacketClass(pkt)
			want := parsePacketClassName(v.Class)
			if got != want {
				t.Fatalf("class=%v want %v (%s)", got, want, v.Class)
			}
		})
	}
}
