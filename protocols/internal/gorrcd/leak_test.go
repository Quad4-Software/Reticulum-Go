// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/internal/leaktest"
)

func TestLeak_RoomRegistryPersist(t *testing.T) {
	base := leaktest.Baseline()
	dir := t.TempDir()
	path := filepath.Join(dir, "rooms.toml")
	reg := NewRoomRegistry(path, 900)
	f := mustID(1)
	for i := range 200 {
		name := "r" + string(rune('a'+i%26))
		_ = reg.Register(name, f)
		reg.SetTopic(name, "t")
		_ = reg.Persist(name)
		_, _ = LoadRoomRegistry(path)
	}
	leaktest.AssertStable(t, base, 12, 5*time.Second)
}

func TestLeak_TrustReload(t *testing.T) {
	base := leaktest.Baseline()
	tr := NewTrust()
	ids := []string{
		"0123456789abcdef0123456789abcdef",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	for range 400 {
		if err := tr.Load(ids[:1], ids[1:]); err != nil {
			t.Fatal(err)
		}
		id := mustID(1)
		tr.Ban(id)
		_ = tr.Unban(id)
		_ = tr.BannedHex()
		_, _ = tr.Counts()
	}
	leaktest.AssertStable(t, base, 8, 4*time.Second)
}

func TestLeak_CommandDispatch(t *testing.T) {
	base := leaktest.Baseline()
	s := NewService(testConfig(), nil)
	peer := mustPeer(7)
	cmds := []string{"/list", "/who lobby", "/nope", "/stats", "/mode lobby +m"}
	for range 300 {
		for _, c := range cmds {
			s.handleCommand(peer, "lobby", c)
		}
	}
	leaktest.AssertStable(t, base, 8, 4*time.Second)
}

func TestLeak_ConfigRoundTrip(t *testing.T) {
	base := leaktest.Baseline()
	dir := t.TempDir()
	path := filepath.Join(dir, "gorrcd.toml")
	cfg := DefaultConfig()
	cfg.HubName = "leak"
	for range 150 {
		if err := WriteConfigFile(cfg, path); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfigFile(path); err != nil {
			t.Fatal(err)
		}
	}
	leaktest.AssertStable(t, base, 10, 4*time.Second)
}
