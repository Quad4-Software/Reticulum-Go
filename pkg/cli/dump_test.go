// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/packet"
)

func TestRunDumpHex(t *testing.T) {
	dest := bytes.Repeat([]byte{0x33}, packet.TruncatedHashLength)
	p := packet.NewPacket(packet.DestinationSingle, []byte("x"), packet.PacketTypeData, packet.ContextNone, packet.PropagationBroadcast, packet.HeaderType1, nil, false, 0)
	p.DestinationHash = dest
	if err := p.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := RunDump([]string{"-hex", hex.EncodeToString(p.Raw)}, Options{Stdout: &stdout, Stderr: &stderr})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"packet_type_name":"DATA"`) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestRunDumpPCAP(t *testing.T) {
	dest := bytes.Repeat([]byte{0x44}, packet.TruncatedHashLength)
	p := packet.NewPacket(packet.DestinationSingle, []byte("y"), packet.PacketTypeAnnounce, packet.ContextPathResponse, packet.PropagationTransport, packet.HeaderType2, bytes.Repeat([]byte{0x55}, 16), false, 0)
	p.DestinationHash = dest
	p.TransportID = bytes.Repeat([]byte{0x55}, 16)
	p.Data = []byte{0x01}
	if err := p.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	path := filepath.Join(t.TempDir(), "one.pcap")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := packet.WritePCAPEthernetUDPv4(f, p.Raw, 4242, 4242); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	var stdout, stderr bytes.Buffer
	code := RunDump([]string{"-pcap", path}, Options{Stdout: &stdout, Stderr: &stderr})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "ANNOUNCE") || !strings.Contains(out, "PATH_RESPONSE") {
		t.Fatalf("stdout=%s", out)
	}
}
