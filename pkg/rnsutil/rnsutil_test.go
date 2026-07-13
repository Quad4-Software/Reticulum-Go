// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/sharedinstance"
	"quad4/reticulum-go/pkg/transport"
)

func TestDestinationHashMatchesDestinationNew(t *testing.T) {
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	want := destination.Hash(id, "exampleapp", "aspect1", "aspect2")
	dest, err := destination.New(id, destination.Out, destination.Single, "exampleapp", nil, "aspect1", "aspect2")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, dest.GetHash()) {
		t.Fatalf("hash mismatch: %x vs %x", want, dest.GetHash())
	}
}

func TestModeNameLabels(t *testing.T) {
	cases := map[byte]string{
		0x01: "Full",
		0x02: "Point-to-Point",
		0x03: "Access Point",
		0x04: "Roaming",
		0x05: "Boundary",
		0x06: "Gateway",
		0x07: "Internal",
	}
	for mode, want := range cases {
		if got := ModeName(mode); got != want {
			t.Errorf("ModeName(0x%02x) = %q, want %q", mode, got, want)
		}
	}
}

func TestParseName(t *testing.T) {
	app, aspects, err := destination.ParseName("app.one.two")
	if err != nil {
		t.Fatal(err)
	}
	if app != "app" || len(aspects) != 2 || aspects[0] != "one" || aspects[1] != "two" {
		t.Fatalf("got %q %v", app, aspects)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	raw := []byte{0x01, 0x02, 0xab, 0xcd}
	for _, enc := range []Encoding{EncHex, EncBase64, EncBase32} {
		s := EncodeBytes(raw, enc)
		out, err := DecodeBytes(s, enc)
		if err != nil {
			t.Fatalf("enc %v: %v", enc, err)
		}
		if !bytes.Equal(raw, out) {
			t.Fatalf("enc %v mismatch", enc)
		}
	}
}

func TestIdentitySignVerifyFile(t *testing.T) {
	dir := t.TempDir()
	idPath := filepath.Join(dir, "id.rid")
	id, err := GenerateIdentity(idPath)
	if err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(dir, "msg.txt")
	if err := os.WriteFile(dataPath, []byte("hello reticulum"), 0o600); err != nil {
		t.Fatal(err)
	}
	sig, err := SignFile(id, dataPath)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyFile(id, dataPath, sig)
	if err != nil || !ok {
		t.Fatalf("verify: ok=%v err=%v", ok, err)
	}
	loaded, err := LoadIdentity(idPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GetHexHash() != id.GetHexHash() {
		t.Fatal("hash mismatch after reload")
	}
}

func TestRPCClientInterfaceStats(t *testing.T) {
	cfg := &common.ReticulumConfig{EnableTransport: false}
	tr := transport.NewTransport(cfg)
	defer tr.Close()

	port := freeTCPPort(t)
	cfg.InstanceControlPort = port
	srv, err := sharedinstance.StartRPCServer(cfg, tr)
	if err != nil {
		t.Fatalf("StartRPCServer: %v", err)
	}
	defer srv.Close()

	client, err := DialRPC(cfg, tr.RPCAuthKey())
	if err != nil {
		t.Fatal(err)
	}
	stats, err := client.GetInterfaceStats()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.TransportID) == 0 {
		t.Fatal("expected transport id")
	}
	n, err := client.GetLinkCount()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("link_count=%d", n)
	}
}

func TestPrettyHexAndSize(t *testing.T) {
	h, _ := hex.DecodeString("aabbccddeeff00112233445566778899")
	p := PrettyHex(h)
	if p[0] != '<' || p[len(p)-1] != '>' {
		t.Fatalf("pretty=%q", p)
	}
	if SizeString(1500, "B") == "" {
		t.Fatal("empty size")
	}
}

func TestWriteStatusJSON(t *testing.T) {
	var buf bytes.Buffer
	stats := transport.InterfaceStatsResponse{
		TransportID: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		RXB:         10,
		TXB:         20,
		Interfaces: []transport.InterfaceStat{{
			Name:   "UDPInterface[test]",
			Status: true,
			RXB:    10,
			TXB:    20,
			Mode:   0,
		}},
	}
	if err := WriteStatusJSON(&buf, stats); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"rxb":10`)) {
		t.Fatalf("json=%s", buf.String())
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestResolveAuthKeyFromRPCKey(t *testing.T) {
	key := bytes.Repeat([]byte{0xab}, 32)
	cfg := &common.ReticulumConfig{RPCKey: key}
	got, err := ResolveAuthKey(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, key) {
		t.Fatal("rpc key mismatch")
	}
}

func TestDialRPCRequiresConfig(t *testing.T) {
	if _, err := DialRPC(nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func BenchmarkDestinationHash(b *testing.B) {
	id, err := identity.NewIdentity()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = destination.Hash(id, "exampleapp", "aspect")
	}
}

func BenchmarkPrettyHex(b *testing.B) {
	h := make([]byte, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PrettyHex(h)
	}
}
