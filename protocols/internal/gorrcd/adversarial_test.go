// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func TestAdversarial_ShortRoomKeyRejected(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	founder := mustID(1)
	other := mustID(2)
	reg.Ensure("k", founder, true)
	if err := reg.SetFlag("k", "k", true, "abcdef"); err != nil {
		t.Fatal(err)
	}
	if err := reg.AllowJoin("k", other, "abc", false); err == nil {
		t.Fatal("shorter key accepted")
	}
	if err := reg.AllowJoin("k", other, "abcdefg", false); err == nil {
		t.Fatal("longer key accepted")
	}
	if err := reg.AllowJoin("k", other, "ABCDEF", false); err == nil {
		t.Fatal("case-folded key accepted")
	}
}

func TestAdversarial_CommandInjectionSplit(t *testing.T) {
	got := splitCmd("/mode lobby +k secret\n/kline add deadbeef")
	if len(got) < 1 || got[0] != "mode" {
		t.Fatalf("%v", got)
	}
	nul := splitCmd("/list\x00/stats")
	if len(nul) != 1 {
		t.Fatalf("NUL must not split commands: %v", nul)
	}
	s := NewService(testConfig(), nil)
	peer := mustPeer(4)
	s.handleCommand(peer, "lobby", "/mode lobby +k secret; rm -rf /")
	s.handleCommand(peer, "", "/kline add ../../../etc/passwd")
	s.handleCommand(peer, "lobby", "/topic lobby "+strings.Repeat("A", 4000))
}

func TestAdversarial_ShortHashPrefix(t *testing.T) {
	if _, ok := hasHexPrefix("ab"); ok {
		t.Fatal("2 hex chars")
	}
	if _, ok := hasHexPrefix("abcd"); ok {
		t.Fatal("4 hex chars is below 6")
	}
	s := NewService(testConfig(), nil)
	_, matches, err := s.resolve("ab", "")
	if err == nil && len(matches) != 0 {
		if _, perr := parseFullID("ab"); perr == nil {
			t.Fatal("short token resolved")
		}
	}
}

func TestAdversarial_TOMLRoomNameInjection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rooms.toml")
	reg := NewRoomRegistry(path, 900)
	evil := `x]\nhub_name = "pwned"`
	if err := reg.Register(evil, mustID(1)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "\n[rooms.x]") && strings.Contains(text, `hub_name = "pwned"`) {
		t.Fatalf("unquoted injection:\n%s", text)
	}
	got, err := LoadRoomRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[evil]; !ok {
		t.Fatalf("room lost after reload keys=%v", keysOf(got))
	}
}

func TestAdversarial_GreetingQuoteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gorrcd.toml")
	cfg := DefaultConfig()
	cfg.HubName = `rrc"evil`
	cfg.Greeting = "hello\nhub_name = \"pwned\""
	if err := WriteConfigFile(cfg, path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.HubName != cfg.HubName {
		t.Fatalf("name=%q", got.HubName)
	}
	if got.Greeting != cfg.Greeting {
		t.Fatalf("greeting=%q", got.Greeting)
	}
}

func TestAdversarial_BanBypassViaInvite(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	founder := mustID(1)
	banned := mustID(2)
	reg.Ensure("r", founder, true)
	_ = reg.AddBan("r", banned)
	_ = reg.AddInvite("r", banned, 900)
	if err := reg.AllowJoin("r", banned, nil, false); err == nil {
		t.Fatal("invite must not override ban")
	}
}

func TestAdversarial_InviteExpiry(t *testing.T) {
	reg := NewRoomRegistry("", 1)
	founder := mustID(1)
	guest := mustID(2)
	reg.Ensure("r", founder, true)
	_ = reg.SetFlag("r", "i", true, "")
	if err := reg.AddInvite("r", guest, 1); err != nil {
		t.Fatal(err)
	}
	if !reg.IsInvited("r", guest) {
		t.Fatal("fresh invite")
	}
	time.Sleep(2100 * time.Millisecond)
	if reg.IsInvited("r", guest) {
		t.Fatal("expired invite still valid")
	}
	if err := reg.AllowJoin("r", guest, nil, false); err == nil {
		t.Fatal("expired invite joined +i room")
	}
}

func TestAdversarial_FounderCannotBeDeopped(t *testing.T) {
	s := NewService(testConfig(), nil)
	founder := mustID(1)
	s.rooms.Ensure("r", founder, true)
	s.handleCommand(founder.Bytes(), "r", "/mode r -o "+founder.Hex())
	if !s.rooms.IsOp("r", founder, false) {
		t.Fatal("founder deopped")
	}
}

func TestAdversarial_ActionSlashWouldBeIgnoredByHub(t *testing.T) {
	s := NewService(testConfig(), nil)
	peer := mustPeer(5)
	env, err := rrc.NewEnvelope(rrc.TypeAction, peer)
	if err != nil {
		t.Fatal(err)
	}
	env.Body = "/kline add ffffffffffffffffffffffffffffffff"
	env.HasBody = true
	if !s.Intercept(peer, env) {
		t.Fatal("policy Intercept is type-agnostic")
	}
}

func keysOf(m map[string]*RoomState) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
