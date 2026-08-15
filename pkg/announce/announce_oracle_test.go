// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package announce

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

type announceWireFile struct {
	IdentityPrvHex  string `json:"identity_prv_hex"`
	IdentityHash    string `json:"identity_hash"`
	IdentityPub     string `json:"identity_pub"`
	SingleDestHash  string `json:"single_dest_hash"`
	NameHash        string `json:"name_hash"`
	RandomHash      string `json:"random_hash"`
	AnnounceSig     string `json:"announce_sig"`
	AnnouncePayload string `json:"announce_payload"`
	PathRequestDest string `json:"path_request_dest"`
}

func loadAnnounceVectors(t *testing.T) announceWireFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "packet", "testdata", "rns_wire_vectors.json"))
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var file announceWireFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return file
}

func TestOracleIdentityAndDestinationHashMatchPythonRNS(t *testing.T) {
	v := loadAnnounceVectors(t)
	prv, err := hex.DecodeString(v.IdentityPrvHex)
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.FromBytes(prv)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	wantHash, _ := hex.DecodeString(v.IdentityHash)
	if !bytes.Equal(id.Hash(), wantHash) {
		t.Fatalf("identity hash=%x want %x", id.Hash(), wantHash)
	}
	wantPub, _ := hex.DecodeString(v.IdentityPub)
	if !bytes.Equal(id.GetPublicKey(), wantPub) {
		t.Fatalf("pub=%x want %x", id.GetPublicKey(), wantPub)
	}
	gotDest := DestinationHash(id, "oracleapp.node")
	wantDest, _ := hex.DecodeString(v.SingleDestHash)
	if !bytes.Equal(gotDest, wantDest) {
		t.Fatalf("dest hash=%x want %x", gotDest, wantDest)
	}
	nameHash := sha256.Sum256([]byte("rnstransport.path.request"))
	prFull := sha256.Sum256(nameHash[:NameHashSize])
	gotPR := prFull[:AddrHashSize]
	wantPR, _ := hex.DecodeString(v.PathRequestDest)
	if !bytes.Equal(gotPR, wantPR) {
		t.Fatalf("path request dest=%x want %x", gotPR, wantPR)
	}
}

func TestOracleAnnouncePayloadLayoutAndSignature(t *testing.T) {
	v := loadAnnounceVectors(t)
	payload, _ := hex.DecodeString(v.AnnouncePayload)
	if len(payload) != PubKeySize+NameHashSize+RandomHashSize+SignatureSize+2 {
		t.Fatalf("payload len=%d", len(payload))
	}
	pub := payload[AnnounceEncKeyOffset : AnnounceEncKeyOffset+PubKeySize]
	nameHash := payload[AnnounceNameHashOffset : AnnounceNameHashOffset+NameHashSize]
	randomHash := payload[AnnounceRandomOffset : AnnounceRandomOffset+RandomHashSize]
	sig := payload[AnnounceRandomOffset+RandomHashSize : AnnounceRandomOffset+RandomHashSize+SignatureSize]
	appData := payload[len(payload)-2:]
	wantPub, _ := hex.DecodeString(v.IdentityPub)
	wantName, _ := hex.DecodeString(v.NameHash)
	wantRand, _ := hex.DecodeString(v.RandomHash)
	wantSig, _ := hex.DecodeString(v.AnnounceSig)
	if !bytes.Equal(pub, wantPub) {
		t.Fatalf("pubkey mismatch")
	}
	if !bytes.Equal(nameHash, wantName) {
		t.Fatalf("name hash mismatch")
	}
	if !bytes.Equal(randomHash, wantRand) {
		t.Fatalf("random hash mismatch")
	}
	if !bytes.Equal(sig, wantSig) {
		t.Fatalf("signature mismatch")
	}
	if !bytes.Equal(appData, []byte{0x01, 0x02}) {
		t.Fatalf("app data=%x", appData)
	}
	destHash, _ := hex.DecodeString(v.SingleDestHash)
	signed := append([]byte{}, destHash...)
	signed = append(signed, pub...)
	signed = append(signed, nameHash...)
	signed = append(signed, randomHash...)
	signed = append(signed, appData...)
	if !cryptography.Verify(ed25519.PublicKey(pub[32:]), signed, sig) {
		t.Fatal("python announce signature failed Go verify")
	}
}

func TestOracleAnnouncePackMatchesPythonRaw(t *testing.T) {
	v := loadAnnounceVectors(t)
	payload, _ := hex.DecodeString(v.AnnouncePayload)
	dest, _ := hex.DecodeString(v.SingleDestHash)
	p := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeAnnounce,
		DestinationType: packet.DestinationSingle,
		DestinationHash: dest,
		Context:         packet.ContextNone,
		Data:            payload,
	}
	if err := p.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "packet", "testdata", "rns_wire_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Packets []struct {
			Name   string `json:"name"`
			RawHex string `json:"raw_hex"`
		} `json:"packets"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	var wantHex string
	for _, pkt := range file.Packets {
		if pkt.Name == "announce_ht1" {
			wantHex = pkt.RawHex
			break
		}
	}
	want, _ := hex.DecodeString(wantHex)
	if !bytes.Equal(p.Raw, want) {
		t.Fatalf("announce pack\n got %x\nwant %x", p.Raw, want)
	}
}

func TestOraclePathResponseContextAndFlags(t *testing.T) {
	v := loadAnnounceVectors(t)
	payload, _ := hex.DecodeString(v.AnnouncePayload)
	dest, _ := hex.DecodeString(v.SingleDestHash)
	tid := bytes.Repeat([]byte{0xab}, AddrHashSize)
	p := &packet.Packet{
		HeaderType:      packet.HeaderType2,
		PacketType:      packet.PacketTypeAnnounce,
		TransportType:   packet.PropagationTransport,
		DestinationType: packet.DestinationSingle,
		DestinationHash: dest,
		TransportID:     tid,
		Context:         packet.ContextPathResponse,
		Data:            payload,
	}
	if err := p.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	if p.Raw[0] != 0x51 {
		t.Fatalf("PATH_RESPONSE flags=%#02x want 0x51", p.Raw[0])
	}
	if err := p.Unpack(); err != nil {
		t.Fatal(err)
	}
	if p.Context != packet.ContextPathResponse {
		t.Fatalf("context=%#02x want PATH_RESPONSE", p.Context)
	}
}

func TestOracleAnnounceMinSizeNoRatchet(t *testing.T) {
	if MinAnnouncePacketSizeNoRatchet != 167 {
		t.Fatalf("MinAnnouncePacketSizeNoRatchet=%d want 167", MinAnnouncePacketSizeNoRatchet)
	}
	v := loadAnnounceVectors(t)
	payload, _ := hex.DecodeString(v.AnnouncePayload)
	if len(payload) < PubKeySize+NameHashSize+RandomHashSize+SignatureSize {
		t.Fatalf("payload shorter than no-ratchet announce data")
	}
}
