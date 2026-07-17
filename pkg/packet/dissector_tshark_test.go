// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/packet"
)

func TestTsharkRNSDissector(t *testing.T) {
	tshark, err := exec.LookPath("tshark")
	if err != nil {
		t.Skip("tshark not installed")
	}

	dest := bytes.Repeat([]byte{0x66}, packet.TruncatedHashLength)
	p := packet.NewPacket(
		packet.DestinationSingle,
		[]byte("dissector"),
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		false,
		0,
	)
	p.DestinationHash = dest
	if err := p.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}

	dir := t.TempDir()
	pcapPath := filepath.Join(dir, "rns.pcap")
	f, err := os.Create(pcapPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := packet.WritePCAPEthernetUDPv4(f, p.Raw, 4242, 4242); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	lua := filepath.Join(filepath.Dir(thisFile), "..", "..", "tools", "wireshark", "rns.lua")
	lua, err = filepath.Abs(lua)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(tshark, "-r", pcapPath, "-X", "lua_script:"+lua, "-T", "fields", "-e", "rns.packet_type", "-e", "rns.context_name", "-Y", "rns")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tshark: %v\n%s", err, out)
	}
	text := string(out)
	// packet_type field is numeric 0 for DATA when using the value string map display may still be 0
	if !strings.Contains(text, "0") && !strings.Contains(strings.ToUpper(text), "DATA") {
		t.Fatalf("unexpected tshark fields output %q raw=%s", text, hex.EncodeToString(p.Raw))
	}
	if !strings.Contains(text, "NONE") {
		t.Fatalf("expected context NONE in %q", text)
	}
}
