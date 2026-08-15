// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/packet"
)

// Smoke and black-box coverage for dump via the public Main dispatcher.
// Snapshot requires a live RPC peer and is covered by interop/self-check.

func TestSmokeDumpViaMain(t *testing.T) {
	dest := bytes.Repeat([]byte{0x21}, packet.TruncatedHashLength)
	p := packet.NewPacket(packet.DestinationSingle, []byte("smoke"), packet.PacketTypeData, packet.ContextNone, packet.PropagationBroadcast, packet.HeaderType1, nil, false, 0)
	p.DestinationHash = dest
	if err := p.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Main([]string{"dump", "-hex", hex.EncodeToString(p.Raw)}, Options{
		Stdout:      &stdout,
		Stderr:      &stderr,
		VersionLine: "test",
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "DATA") {
		t.Fatalf("stdout missing DATA: %s", stdout.String())
	}
}

func TestBlackBoxDumpHelpAndVersion(t *testing.T) {
	var out bytes.Buffer
	code := Main([]string{"dump", "-h"}, Options{Stdout: &out, Stderr: &out, VersionLine: "v-test"})
	if code != 0 && code != 2 {
		t.Fatalf("dump -h code=%d out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "-hex") {
		t.Fatalf("dump help missing -hex: %s", out.String())
	}
	out.Reset()
	if code := Main([]string{"--version"}, Options{Stdout: &out, Stderr: &out, VersionLine: "v-test"}); code != 0 {
		t.Fatalf("version code=%d", code)
	}
	if !strings.Contains(out.String(), "v-test") {
		t.Fatalf("version out=%s", out.String())
	}
}

func TestBlackBoxRgodumpAlias(t *testing.T) {
	dest := bytes.Repeat([]byte{0x22}, packet.TruncatedHashLength)
	p := packet.NewPacket(packet.DestinationSingle, []byte("bb"), packet.PacketTypeData, packet.ContextNone, packet.PropagationBroadcast, packet.HeaderType1, nil, false, 0)
	p.DestinationHash = dest
	if err := p.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := Main([]string{"-hex", hex.EncodeToString(p.Raw)}, Options{
		Stdout:      &stdout,
		Stderr:      &stderr,
		Argv0:       "rgodump",
		VersionLine: "test",
	})
	if code != 0 {
		t.Fatalf("rgodump code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "DATA") {
		t.Fatalf("stdout=%s", stdout.String())
	}
}
