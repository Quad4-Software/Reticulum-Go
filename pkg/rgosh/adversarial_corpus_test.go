// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type adversarialManifest struct {
	FormatVersion int               `json:"format_version"`
	Cases         []adversarialCase `json:"cases"`
}

type adversarialCase struct {
	Name          string `json:"name"`
	MsgTypeHex    string `json:"msg_type_hex"`
	RawHex        string `json:"raw_hex"`
	Expect        string `json:"expect"`
	ErrorContains string `json:"error_contains"`
}

func TestAdversarialCorpus(t *testing.T) {
	dir := filepath.Join("testdata", "adversarial")
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var man adversarialManifest
	if err := json.Unmarshal(raw, &man); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if man.FormatVersion < 1 || len(man.Cases) == 0 {
		t.Fatal("bad manifest")
	}
	for _, c := range man.Cases {
		t.Run(c.Name, func(t *testing.T) {
			mt, err := hex.DecodeString(c.MsgTypeHex)
			if err != nil || len(mt) != 2 {
				t.Fatalf("msg_type_hex: %v", err)
			}
			msgType := uint16(mt[0])<<8 | uint16(mt[1])
			body, err := hex.DecodeString(c.RawHex)
			if err != nil {
				t.Fatalf("raw_hex: %v", err)
			}
			_, err = UnpackMessage(msgType, body)
			if c.Expect == "ok" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if c.ErrorContains != "" && !strings.Contains(err.Error(), c.ErrorContains) {
				t.Fatalf("err=%v want contains %q", err, c.ErrorContains)
			}
		})
	}
}

func FuzzNativeUnpack(f *testing.F) {
	f.Add(uint16(NativeVersion), []byte{0, 1, 0, 0, 0, 1, 'x'})
	f.Add(uint16(NativeExec), []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Add(uint16(NativeStream), []byte{0, 1, 'a', 'b'})
	f.Fuzz(func(t *testing.T, msgType uint16, body []byte) {
		msgType = NativeNoop&0xff00 | (msgType & 0x00ff)
		_, _ = UnpackMessage(msgType, body)
	})
}

func FuzzCompatUnpack(f *testing.F) {
	f.Add(uint16(CompatVersion), mustDecode("92a5312e302e3001"))
	f.Add(uint16(CompatExec), mustDecode("9a92a92f62696e2f6563686fa26869c3c3c3c0a5787465726d18500000"))
	f.Fuzz(func(t *testing.T, msgType uint16, body []byte) {
		msgType = CompatNoop&0xff00 | (msgType & 0x00ff)
		_, _ = UnpackMessage(msgType, body)
	})
}

func FuzzStreamInflate(f *testing.F) {
	f.Add([]byte{0x40, 0x01, 1, 2, 3, 4})
	f.Fuzz(func(t *testing.T, body []byte) {
		msg := &StreamMessage{}
		_ = msg.Unpack(body)
	})
}

func mustDecode(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return b
}
