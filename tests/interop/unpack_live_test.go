// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/packet"
)

func pythonUnpackOK(t *testing.T, raw []byte) bool {
	t.Helper()
	line := strings.TrimSpace(string(runUnpackOracle(t, raw)))
	if strings.Contains(line, "UNPACK_OK") {
		return true
	}
	if strings.Contains(line, "UNPACK_FAIL") {
		return false
	}
	t.Fatalf("python unpack oracle out=%q", line)
	return false
}

func runUnpackOracle(t *testing.T, raw []byte) []byte {
	t.Helper()
	args := []string{pyScript(t, "unpack_oracle.py")}
	var stdin *strings.Reader
	if len(raw) > 2048 {
		args = append(args, "-")
		stdin = strings.NewReader(hexOf(raw))
	} else {
		args = append(args, hexOf(raw))
	}
	cmd := exec.Command(pythonExe(), args...)
	cmd.Env = os.Environ()
	if stdin != nil {
		cmd.Stdin = stdin
	}
	out, err := cmd.CombinedOutput()
	if err != nil && !bytes.Contains(out, []byte("UNPACK_FAIL")) {
		t.Fatalf("python unpack oracle: err=%v out=%q", err, out)
	}
	return out
}

func pythonRoundtrip(t *testing.T, raw []byte) []byte {
	t.Helper()
	args := []string{pyScript(t, "unpack_oracle.py"), "roundtrip"}
	var stdin *strings.Reader
	if len(raw) > 2048 {
		args = append(args, "-")
		stdin = strings.NewReader(hexOf(raw))
	} else {
		args = append(args, hexOf(raw))
	}
	cmd := exec.Command(pythonExe(), args...)
	cmd.Env = os.Environ()
	if stdin != nil {
		cmd.Stdin = stdin
	}
	out, err := cmd.CombinedOutput()
	line := strings.TrimSpace(string(out))
	if after, ok := strings.CutPrefix(line, "PACKED "); ok {
		got, decErr := hex.DecodeString(after)
		if decErr != nil {
			t.Fatalf("packed hex: %v", decErr)
		}
		return got
	}
	t.Fatalf("python roundtrip: err=%v out=%q", err, line)
	return nil
}

func hexOf(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexdigits[v>>4]
		out[i*2+1] = hexdigits[v&0x0f]
	}
	return string(out)
}

func ht1(hops byte, dest, data []byte) []byte {
	raw := make([]byte, 0, packet.MinPacketSize+len(dest)+len(data))
	raw = append(raw, 0x00, hops)
	raw = append(raw, dest...)
	raw = append(raw, packet.ContextNone)
	return append(raw, data...)
}

func ht2(hops byte, tid, dest, data []byte) []byte {
	flags := byte(packet.HeaderType2 << 6)
	raw := make([]byte, 0, packet.MinPacketSize+2*packet.TruncatedHashLength+len(data))
	raw = append(raw, flags, hops)
	raw = append(raw, tid...)
	raw = append(raw, dest...)
	raw = append(raw, packet.ContextNone)
	return append(raw, data...)
}

func TestLiveInteropUnpackOracleMatchesPython(t *testing.T) {
	liveOrSkip(t)
	dest := bytes.Repeat([]byte{0x11}, packet.TruncatedHashLength)
	tid := bytes.Repeat([]byte{0x22}, packet.TruncatedHashLength)
	fullHT2 := ht2(1, tid, dest, []byte("x"))
	need := 2*packet.TruncatedHashLength + packet.MinPacketSize
	cases := []struct {
		name string
		raw  []byte
		ok   bool
	}{
		{"hops127", ht1(packet.PathfinderM-1, dest, []byte("ok")), true},
		{"hops128", ht1(packet.PathfinderM, dest, []byte("no")), false},
		{"hops255", ht1(255, dest, []byte("no")), false},
		{"short", []byte{0x00, 0x00}, false},
		{"oversize", make([]byte, packet.MaxInboundPacketSize+1), false},
		{"ht2ok", fullHT2, true},
		{"ht2trunc", fullHT2[:need-1], false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &packet.Packet{Raw: append([]byte(nil), tc.raw...)}
			goOK := p.Unpack() == nil
			if goOK != tc.ok {
				t.Fatalf("go unpack ok=%v want %v", goOK, tc.ok)
			}
			if tc.name == "oversize" {
				return
			}
			pyOK := pythonUnpackOK(t, tc.raw)
			if pyOK != goOK {
				t.Fatalf("python ok=%v go ok=%v", pyOK, goOK)
			}
		})
	}
}

func TestLiveInteropPackUnpackByteIdentity(t *testing.T) {
	liveOrSkip(t)
	dest := bytes.Repeat([]byte{0x11}, packet.TruncatedHashLength)
	tid := bytes.Repeat([]byte{0x22}, packet.TruncatedHashLength)
	cases := []struct {
		name string
		pkt  *packet.Packet
	}{
		{
			name: "ht1data",
			pkt: &packet.Packet{
				HeaderType:      packet.HeaderType1,
				PacketType:      packet.PacketTypeData,
				DestinationType: packet.DestinationSingle,
				DestinationHash: append([]byte(nil), dest...),
				Data:            []byte("hello-data"),
			},
		},
		{
			name: "ht2transport",
			pkt: &packet.Packet{
				HeaderType:      packet.HeaderType2,
				PacketType:      packet.PacketTypeData,
				TransportType:   packet.PropagationTransport,
				DestinationType: packet.DestinationSingle,
				DestinationHash: append([]byte(nil), dest...),
				TransportID:     append([]byte(nil), tid...),
				Data:            []byte("ht2-body"),
			},
		},
		{
			name: "announce",
			pkt: &packet.Packet{
				HeaderType:      packet.HeaderType1,
				PacketType:      packet.PacketTypeAnnounce,
				DestinationType: packet.DestinationSingle,
				DestinationHash: append([]byte(nil), dest...),
				Data:            bytes.Repeat([]byte{0xab}, 170),
			},
		},
		{
			name: "proof",
			pkt: &packet.Packet{
				HeaderType:      packet.HeaderType1,
				PacketType:      packet.PacketTypeProof,
				DestinationType: packet.DestinationSingle,
				DestinationHash: append([]byte(nil), dest...),
				Data:            bytes.Repeat([]byte{0xcd}, 32),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.pkt.Pack(); err != nil {
				t.Fatal(err)
			}
			py := pythonRoundtrip(t, tc.pkt.Raw)
			if !bytes.Equal(py, tc.pkt.Raw) {
				t.Fatalf("python pack != go pack\n go=%x\n py=%x", tc.pkt.Raw, py)
			}
			back := &packet.Packet{Raw: append([]byte(nil), py...)}
			if err := back.Unpack(); err != nil {
				t.Fatalf("re-unpack: %v", err)
			}
		})
	}
}
