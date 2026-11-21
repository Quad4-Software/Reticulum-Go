// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIdentityHash(t *testing.T) {
	raw := "0123456789abcdef0123456789abcdef"
	b, err := parseIdentityHash(raw)
	if err != nil || len(b) != 16 {
		t.Fatalf("parse: %v %x", err, b)
	}
	id, err := parseFullID("0x" + raw)
	if err != nil || id.Hex() != raw {
		t.Fatalf("full: %v %s", err, id.Hex())
	}
	if _, err := parseIdentityHash("ab"); err == nil {
		t.Fatal("short hash accepted")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gorrcd.toml")
	cfg := DefaultConfig()
	cfg.HubName = "test-hub"
	cfg.TrustedIdentities = []string{"0123456789abcdef0123456789abcdef"}
	cfg.BannedIdentities = []string{"ffffffffffffffffffffffffffffffff"}
	cfg.Greeting = "hello"
	if err := WriteConfigFile(cfg, path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.HubName != "test-hub" || got.Greeting != "hello" {
		t.Fatalf("name/greeting: %+v", got)
	}
	if len(got.TrustedIdentities) != 1 || len(got.BannedIdentities) != 1 {
		t.Fatalf("identities: %+v", got)
	}
	if err := persistBannedIdentities(path, []string{"aa", "bb"}); err != nil {
		t.Fatal(err)
	}
	got2, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got2.BannedIdentities) != 2 {
		t.Fatalf("banned after persist: %v", got2.BannedIdentities)
	}
}

func TestRoomJoinPolicy(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	var founder ID
	copy(founder[:], []byte("0123456789abcdef"))
	var other ID
	copy(other[:], []byte("abcdef0123456789"))
	reg.Ensure("lobby", founder, true)
	if err := reg.AllowJoin("lobby", other, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := reg.SetFlag("lobby", "i", true, ""); err != nil {
		t.Fatal(err)
	}
	if err := reg.AllowJoin("lobby", other, nil, false); err == nil {
		t.Fatal("invite-only should reject")
	}
	_ = reg.AddInvite("lobby", other, 900)
	if err := reg.AllowJoin("lobby", other, nil, false); err != nil {
		t.Fatal(err)
	}
	reg.ConsumeInvite("lobby", other)
	if err := reg.SetFlag("lobby", "i", false, ""); err != nil {
		t.Fatal(err)
	}
	if err := reg.SetFlag("lobby", "k", true, "secret"); err != nil {
		t.Fatal(err)
	}
	if err := reg.AllowJoin("lobby", other, "nope", false); err == nil {
		t.Fatal("bad key should reject")
	}
	if err := reg.AllowJoin("lobby", other, "secret", false); err != nil {
		t.Fatal(err)
	}
	_ = reg.AddBan("lobby", other)
	if err := reg.AllowJoin("lobby", other, "secret", false); err == nil {
		t.Fatal("ban should reject")
	}
}

func TestRoomContentPolicy(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	var peer ID
	copy(peer[:], []byte("0123456789abcdef"))
	reg.Ensure("mod", peer, true)
	_ = reg.SetFlag("mod", "m", true, "")
	if err := reg.AllowContent("mod", peer, true, false); err != nil {
		t.Fatal("founder is voiced")
	}
	var other ID
	copy(other[:], []byte("abcdef0123456789"))
	if err := reg.AllowContent("mod", other, true, false); err == nil {
		t.Fatal("unvoiced should fail +m")
	}
	_ = reg.SetFlag("mod", "n", true, "")
	if err := reg.AllowContent("mod", other, false, false); err == nil {
		t.Fatal("+n should reject outsider")
	}
}

func TestFirstRun(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "gorrcd.toml")
	id := filepath.Join(dir, "hub_identity")
	rooms := filepath.Join(dir, "rooms.toml")
	rns := filepath.Join(dir, "rns")
	created, err := FirstRun(cfg, id, rooms, rns)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if _, err := os.Stat(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(rns, "config")); err != nil {
		t.Fatal(err)
	}
	created2, err := FirstRun(cfg, id, rooms, rns)
	if err != nil || created2 {
		t.Fatalf("second run created=%v err=%v", created2, err)
	}
}

func TestTrustBan(t *testing.T) {
	tr := NewTrust()
	raw := "0123456789abcdef0123456789abcdef"
	if err := tr.Load([]string{raw}, nil); err != nil {
		t.Fatal(err)
	}
	id, _ := parseFullID(raw)
	if !tr.IsTrusted(id) {
		t.Fatal("trusted")
	}
	tr.Ban(id)
	if !tr.IsBanned(id) {
		t.Fatal("banned")
	}
	if !tr.Unban(id) {
		t.Fatal("unban")
	}
}

func TestSplitCmd(t *testing.T) {
	got := splitCmd("/mode lobby +k secret key")
	if len(got) < 4 || got[0] != "mode" {
		t.Fatalf("%v", got)
	}
}

func TestModeString(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	var f ID
	copy(f[:], []byte("0123456789abcdef"))
	reg.Ensure("x", f, true)
	_ = reg.SetFlag("x", "m", true, "")
	_ = reg.SetFlag("x", "n", true, "")
	s := reg.ModeString("x")
	if s != "+mn" {
		t.Fatalf("mode=%s", s)
	}
}

func TestRoomRegistryPersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rooms.toml")
	reg := NewRoomRegistry(path, 900)
	var f ID
	copy(f[:], []byte("0123456789abcdef"))
	if err := reg.Register("lobby", f); err != nil {
		t.Fatal(err)
	}
	reg.SetTopic("lobby", "hello")
	if err := reg.Persist("lobby"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadRoomRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	st := got["lobby"]
	if st == nil || !st.Registered || st.Topic != "hello" || !st.HasFounder {
		t.Fatalf("loaded: %+v", st)
	}
	if st.Founder != f {
		t.Fatalf("founder %x", st.Founder)
	}
}

func TestUnknownCommandNoPanic(t *testing.T) {
	s := NewService(DefaultConfig(), nil)
	peer := make([]byte, 16)
	copy(peer, []byte("0123456789abcdef"))
	s.handleCommand(peer, "lobby", "/nope")
	s.handleCommand(peer, "", "/stats")
}

func TestUDPAddr(t *testing.T) {
	if udpAddr("42950") != "127.0.0.1:42950" {
		t.Fatal(udpAddr("42950"))
	}
	if udpAddr("127.0.0.1:9") != "127.0.0.1:9" {
		t.Fatal(udpAddr("127.0.0.1:9"))
	}
}

func TestSetFlagRejectsEmptyKey(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	if err := reg.SetFlag("r", "k", true, ""); err == nil {
		t.Fatal("empty key accepted")
	}
}
