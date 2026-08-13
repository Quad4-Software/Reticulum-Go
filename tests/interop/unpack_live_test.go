// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/packet"
)

func pythonUnpackOK(t *testing.T, raw []byte) bool {
	t.Helper()
	cmd := exec.Command(pythonExe(), pyScript(t, "unpack_oracle.py"), hexOf(raw))
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	line := strings.TrimSpace(string(out))
	if strings.Contains(line, "UNPACK_OK") {
		return true
	}
	if strings.Contains(line, "UNPACK_FAIL") {
		return false
	}
	t.Fatalf("python unpack oracle: err=%v out=%q", err, line)
	return false
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

func TestLiveInteropUnpackOracleMatchesPython(t *testing.T) {
	liveOrSkip(t)
	dest := bytes.Repeat([]byte{0x11}, packet.TruncatedHashLength)
	cases := []struct {
		name string
		raw  []byte
		ok   bool
	}{
		{"hops127", ht1(packet.PathfinderM-1, dest, []byte("ok")), true},
		{"hops128", ht1(packet.PathfinderM, dest, []byte("no")), false},
		{"hops255", ht1(255, dest, []byte("no")), false},
		{"short", []byte{0x00, 0x00}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &packet.Packet{Raw: append([]byte(nil), tc.raw...)}
			goOK := p.Unpack() == nil
			if goOK != tc.ok {
				t.Fatalf("go unpack ok=%v want %v", goOK, tc.ok)
			}
			pyOK := pythonUnpackOK(t, tc.raw)
			if pyOK != goOK {
				t.Fatalf("python ok=%v go ok=%v", pyOK, goOK)
			}
		})
	}
}
