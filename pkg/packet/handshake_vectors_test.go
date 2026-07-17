// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

type handshakeVector struct {
	Name           string       `json:"name"`
	RawHex         string       `json:"raw_hex"`
	PacketTypeName string       `json:"packet_type_name"`
	ContextName    string       `json:"context_name"`
	HeaderType     byte         `json:"header_type"`
	DestTypeName   string       `json:"destination_type_name"`
	Tree           packet.Frame `json:"tree"`
}

// TestHandshakeVectorPackAndReplay builds known-good announce, PATH_RESPONSE,
// and LINKREQUEST frames then checks DecodeFrame trees. Vectors are also
// written under testdata for external porters.
func TestHandshakeVectorPackAndReplay(t *testing.T) {
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	dest := id.Hash()
	if len(dest) > packet.TruncatedHashLength {
		dest = dest[:packet.TruncatedHashLength]
	}
	transportID := bytes.Repeat([]byte{0xab}, packet.TruncatedHashLength)

	announce, err := packet.NewAnnouncePacket(dest, id, []byte{0x01, 0x02}, transportID)
	if err != nil {
		t.Fatalf("announce: %v", err)
	}
	if err := announce.Pack(); err != nil {
		t.Fatalf("announce pack: %v", err)
	}

	pathResp := &packet.Packet{
		DestinationType: packet.DestinationSingle,
		PacketType:      packet.PacketTypeAnnounce,
		Context:         packet.ContextPathResponse,
		HeaderType:      packet.HeaderType2,
		TransportType:   packet.PropagationTransport,
		DestinationHash: append([]byte(nil), dest...),
		TransportID:     append([]byte(nil), transportID...),
		Data:            append([]byte(nil), announce.Data...),
	}
	if err := pathResp.Pack(); err != nil {
		t.Fatalf("path resp pack: %v", err)
	}

	linkReq := packet.NewPacket(
		packet.DestinationSingle,
		bytes.Repeat([]byte{0xcd}, packet.LinkRequestECPubSize),
		packet.PacketTypeLinkReq,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		false,
		0,
	)
	linkReq.DestinationHash = append([]byte(nil), dest...)
	if err := linkReq.Pack(); err != nil {
		t.Fatalf("link req pack: %v", err)
	}

	vectors := []handshakeVector{
		mustVector(t, "announce_h2", announce),
		mustVector(t, "path_response_announce", pathResp),
		mustVector(t, "link_request", linkReq),
	}

	for _, v := range vectors {
		raw, err := hex.DecodeString(v.RawHex)
		if err != nil {
			t.Fatalf("%s hex: %v", v.Name, err)
		}
		fr := packet.DecodeFrame(raw)
		if !fr.OK {
			t.Fatalf("%s decode: %s", v.Name, fr.Error)
		}
		if fr.PacketTypeName != v.PacketTypeName {
			t.Fatalf("%s type %s want %s", v.Name, fr.PacketTypeName, v.PacketTypeName)
		}
		if fr.ContextName != v.ContextName {
			t.Fatalf("%s ctx %s want %s", v.Name, fr.ContextName, v.ContextName)
		}
	}

	outDir := filepath.Join("testdata")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body, err := json.MarshalIndent(map[string]any{
		"format_version": 1,
		"vectors":        vectors,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(outDir, "handshake_vectors.json")
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Reload and replay from disk so the checked-in artifact stays valid.
	rawFile, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var file struct {
		Vectors []handshakeVector `json:"vectors"`
	}
	if err := json.Unmarshal(rawFile, &file); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(file.Vectors) != len(vectors) {
		t.Fatalf("vector count %d", len(file.Vectors))
	}
	for _, v := range file.Vectors {
		raw, _ := hex.DecodeString(v.RawHex)
		fr := packet.DecodeFrame(raw)
		if !fr.OK || fr.PacketTypeName != v.PacketTypeName {
			t.Fatalf("replay %s failed", v.Name)
		}
	}
}

func mustVector(t *testing.T, name string, p *packet.Packet) handshakeVector {
	t.Helper()
	fr := packet.DecodeFrame(p.Raw)
	if !fr.OK {
		t.Fatalf("%s: %s", name, fr.Error)
	}
	return handshakeVector{
		Name:           name,
		RawHex:         hex.EncodeToString(p.Raw),
		PacketTypeName: fr.PacketTypeName,
		ContextName:    fr.ContextName,
		HeaderType:     fr.HeaderType,
		DestTypeName:   fr.DestTypeName,
		Tree:           fr,
	}
}
