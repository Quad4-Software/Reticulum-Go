// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzParseIdentityHash(f *testing.F) {
	f.Add("0123456789abcdef0123456789abcdef")
	f.Add("0x0123456789abcdef0123456789abcdef")
	f.Add("ab")
	f.Add("")
	f.Add("zzzz")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			s = s[:4096]
		}
		b, err := parseIdentityHash(s)
		if err != nil {
			return
		}
		if len(b) < 4 {
			t.Fatal("short hash accepted")
		}
		id, err := parseFullID(s)
		if err != nil {
			return
		}
		compact := strings.ReplaceAll(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x"), " ", "")
		compact = strings.ReplaceAll(compact, "\t", "")
		if len(b) == 16 && id.Hex() != compact {
			t.Fatalf("hex drift %s vs %s", id.Hex(), compact)
		}
	})
}

func FuzzSplitCmd(f *testing.F) {
	f.Add("/list")
	f.Add("/mode lobby +k secret")
	f.Add("")
	f.Add("/")
	f.Add("/who #Lobby extra")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			s = s[:4096]
		}
		got := splitCmd(s)
		for _, p := range got {
			if p == "" {
				t.Fatal("empty token")
			}
		}
	})
}

func FuzzParseRoomsTOML(f *testing.F) {
	f.Add("# empty\n")
	f.Add("[rooms]\n\n[rooms.lobby]\nfounder = \"0123456789abcdef0123456789abcdef\"\nmoderated = true\n")
	f.Add("[rooms.\"#ops\"]\ntopic = \"hi\"\n")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 64<<10 {
			s = s[:64<<10]
		}
		_, _ = parseRoomsTOML(s)
	})
}

func TestMutation_RoomsTOMLCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rooms.toml")
	reg := NewRoomRegistry(path, 900)
	if err := reg.Register("lobby", mustID(1)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("empty")
	}
	raw[len(raw)/2] ^= 0xff
	_, err = parseRoomsTOML(string(raw))
	if err != nil {
		return
	}
}
