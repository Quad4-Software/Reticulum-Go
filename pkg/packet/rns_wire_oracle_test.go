// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type rnsWireFile struct {
	Packets []rnsWirePacket `json:"packets"`
}

type rnsWirePacket struct {
	Name            string `json:"name"`
	RawHex          string `json:"raw_hex"`
	HashHex         string `json:"hash_hex"`
	Hops            byte   `json:"hops"`
	Context         byte   `json:"context"`
	Flags           byte   `json:"flags"`
	HeaderType      byte   `json:"header_type"`
	PacketType      byte   `json:"packet_type"`
	DestinationType byte   `json:"destination_type"`
	TransportType   byte   `json:"transport_type"`
}

func loadRNSWireVectors(t *testing.T) rnsWireFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "rns_wire_vectors.json"))
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var file rnsWireFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("unmarshal vectors: %v", err)
	}
	if len(file.Packets) == 0 {
		t.Fatal("no packet vectors")
	}
	return file
}

func TestOraclePackedFlagsMatchPythonRNS(t *testing.T) {
	for ht := byte(0); ht <= 1; ht++ {
		for cf := byte(0); cf <= 1; cf++ {
			for tt := byte(0); tt <= 1; tt++ {
				for dt := byte(0); dt <= 3; dt++ {
					for pt := byte(0); pt <= 3; pt++ {
						p := &Packet{
							HeaderType:      ht,
							ContextFlag:     cf,
							TransportType:   tt,
							DestinationType: dt,
							PacketType:      pt,
						}
						got := p.headerFlags()
						want := (ht << 6) | (cf << 5) | (tt << 4) | (dt << 2) | pt
						if got != want {
							t.Fatalf("flags ht=%d cf=%d tt=%d dt=%d pt=%d got %d want %d", ht, cf, tt, dt, pt, got, want)
						}
					}
				}
			}
		}
	}
}

func TestOracleContextBytesMatchPythonRNS(t *testing.T) {
	cases := []struct {
		name string
		got  byte
		want byte
	}{
		{"NONE", ContextNone, 0x00},
		{"RESOURCE", ContextResource, 0x01},
		{"RESOURCE_ADV", ContextResourceAdv, 0x02},
		{"RESOURCE_REQ", ContextResourceReq, 0x03},
		{"RESOURCE_HMU", ContextResourceHMU, 0x04},
		{"RESOURCE_PRF", ContextResourcePRF, 0x05},
		{"RESOURCE_ICL", ContextResourceICL, 0x06},
		{"RESOURCE_RCL", ContextResourceRCL, 0x07},
		{"CACHE_REQUEST", ContextCacheReq, 0x08},
		{"REQUEST", ContextRequest, 0x09},
		{"RESPONSE", ContextResponse, 0x0A},
		{"PATH_RESPONSE", ContextPathResponse, 0x0B},
		{"COMMAND", ContextCommand, 0x0C},
		{"COMMAND_STATUS", ContextCmdStatus, 0x0D},
		{"CHANNEL", ContextChannel, 0x0E},
		{"KEEPALIVE", ContextKeepalive, 0xFA},
		{"LINKIDENTIFY", ContextLinkIdentify, 0xFB},
		{"LINKCLOSE", ContextLinkClose, 0xFC},
		{"LINKPROOF", ContextLinkProof, 0xFD},
		{"LRRTT", ContextLRRTT, 0xFE},
		{"LRPROOF", ContextLRProof, 0xFF},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("%s context=%#02x want %#02x", tc.name, tc.got, tc.want)
		}
	}
}

func TestOracleMDUMatchesPythonRNS(t *testing.T) {
	if MTU != 500 {
		t.Fatalf("MTU=%d want 500", MTU)
	}
	if HeaderType1Overhead != 19 {
		t.Fatalf("HEADER_MINSIZE=%d want 19", HeaderType1Overhead)
	}
	if EncryptedMDU != 383 {
		t.Fatalf("ENCRYPTED_MDU=%d want 383", EncryptedMDU)
	}
	if PlainMDU != 464 {
		t.Fatalf("PLAIN_MDU=%d want 464", PlainMDU)
	}
}

func TestOraclePythonWireVectorsUnpackAndHash(t *testing.T) {
	file := loadRNSWireVectors(t)
	for _, v := range file.Packets {
		t.Run(v.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(v.RawHex)
			if err != nil {
				t.Fatalf("hex: %v", err)
			}
			p := &Packet{Raw: append([]byte(nil), raw...)}
			if err := p.Unpack(); err != nil {
				t.Fatalf("unpack: %v", err)
			}
			if p.Hops != v.Hops {
				t.Fatalf("hops=%d want %d", p.Hops, v.Hops)
			}
			if p.Context != v.Context {
				t.Fatalf("context=%#02x want %#02x", p.Context, v.Context)
			}
			if p.HeaderType != v.HeaderType {
				t.Fatalf("header_type=%d want %d", p.HeaderType, v.HeaderType)
			}
			if p.PacketType != v.PacketType {
				t.Fatalf("packet_type=%d want %d", p.PacketType, v.PacketType)
			}
			if p.DestinationType != v.DestinationType {
				t.Fatalf("dest_type=%d want %d", p.DestinationType, v.DestinationType)
			}
			if p.TransportType != v.TransportType {
				t.Fatalf("transport_type=%d want %d", p.TransportType, v.TransportType)
			}
			if raw[0] != v.Flags {
				t.Fatalf("flags=%d want %d", raw[0], v.Flags)
			}
			wantHash, err := hex.DecodeString(v.HashHex)
			if err != nil {
				t.Fatalf("hash hex: %v", err)
			}
			if !bytes.Equal(p.GetHash(), wantHash) {
				t.Fatalf("hash=%x want %x", p.GetHash(), wantHash)
			}
			fr := DecodeFrame(raw)
			if !fr.OK {
				t.Fatalf("DecodeFrame: %s", fr.Error)
			}
			if fr.Context != v.Context {
				t.Fatalf("tree context=%#02x want %#02x", fr.Context, v.Context)
			}
		})
	}
}

func TestOraclePythonWireVectorsRepack(t *testing.T) {
	file := loadRNSWireVectors(t)
	for _, v := range file.Packets {
		t.Run(v.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(v.RawHex)
			if err != nil {
				t.Fatalf("hex: %v", err)
			}
			p := &Packet{Raw: append([]byte(nil), raw...)}
			if err := p.Unpack(); err != nil {
				t.Fatalf("unpack: %v", err)
			}
			p.Packed = false
			p.Raw = nil
			if err := p.Pack(); err != nil {
				t.Fatalf("pack: %v", err)
			}
			if !bytes.Equal(p.Raw, raw) {
				t.Fatalf("repack\n got %x\nwant %x", p.Raw, raw)
			}
		})
	}
}

func TestOracleHopIncrementOnlyTouchesHopsByte(t *testing.T) {
	file := loadRNSWireVectors(t)
	var v rnsWirePacket
	for _, p := range file.Packets {
		if p.Name == "data_hops3" {
			v = p
			break
		}
	}
	if v.Name == "" {
		t.Fatal("missing data_hops3 vector")
	}
	raw, err := hex.DecodeString(v.RawHex)
	if err != nil {
		t.Fatal(err)
	}
	p := &Packet{Raw: append([]byte(nil), raw...)}
	if err := p.Unpack(); err != nil {
		t.Fatal(err)
	}
	if p.Hops != 3 {
		t.Fatalf("hops=%d want 3", p.Hops)
	}
	p.Hops = 4
	p.Packed = false
	p.Raw = nil
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}
	if p.Raw[1] != 4 {
		t.Fatalf("packed hops byte=%d want 4", p.Raw[1])
	}
	want := append([]byte(nil), raw...)
	want[1] = 4
	if !bytes.Equal(p.Raw, want) {
		t.Fatalf("hop increment mutated more than hops byte")
	}
}

func TestOraclePathRequestIsPlainData(t *testing.T) {
	file := loadRNSWireVectors(t)
	var v rnsWirePacket
	for _, p := range file.Packets {
		if p.Name == "path_request" {
			v = p
			break
		}
	}
	if v.Name == "" {
		t.Fatal("missing path_request vector")
	}
	raw, _ := hex.DecodeString(v.RawHex)
	p := &Packet{Raw: raw}
	if err := p.Unpack(); err != nil {
		t.Fatal(err)
	}
	if p.PacketType != PacketTypeData {
		t.Fatalf("path request type=%d want DATA", p.PacketType)
	}
	if p.DestinationType != DestinationPlain {
		t.Fatalf("path request dest type=%d want PLAIN", p.DestinationType)
	}
	if len(p.Data) != 32 {
		t.Fatalf("path request payload len=%d want 32 (dest+tag)", len(p.Data))
	}
}

func rnsPythonOrSkip(t *testing.T) string {
	t.Helper()
	exe := os.Getenv("PYTHON_INTEROP")
	if exe == "" {
		exe = "python3"
	}
	cmd := exec.Command(exe, "-c", "from RNS.Packet import Packet")
	if err := cmd.Run(); err != nil {
		if os.Getenv("RUN_PY_INTEROP") != "" {
			t.Fatalf("python RNS Packet required: %v", err)
		}
		t.Skip("python RNS not available")
	}
	return exe
}

func TestPythonRNSUnpacksGoPackedVectors(t *testing.T) {
	exe := rnsPythonOrSkip(t)
	script := filepath.Join("..", "..", "tests", "interop", "py", "unpack_oracle.py")
	file := loadRNSWireVectors(t)
	for _, v := range file.Packets {
		t.Run(v.Name, func(t *testing.T) {
			cmd := exec.Command(exe, script, "roundtrip", v.RawHex)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("python roundtrip: %v\n%s", err, out)
			}
			line := strings.TrimSpace(string(out))
			if !strings.HasPrefix(line, "PACKED ") {
				t.Fatalf("python output %q", line)
			}
			got, err := hex.DecodeString(strings.TrimPrefix(line, "PACKED "))
			if err != nil {
				t.Fatalf("python hex: %v", err)
			}
			want, _ := hex.DecodeString(v.RawHex)
			if !bytes.Equal(got, want) {
				t.Fatalf("python packed\n got %x\nwant %x", got, want)
			}
		})
	}
}
