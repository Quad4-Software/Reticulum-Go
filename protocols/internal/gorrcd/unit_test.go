// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func TestUnit_ParseIdentityHash(t *testing.T) {
	raw := "0123456789abcdef0123456789abcdef"
	b, err := parseIdentityHash(raw)
	if err != nil || len(b) != 16 {
		t.Fatalf("parse: %v %x", err, b)
	}
	id, err := parseFullID("0x" + raw)
	if err != nil || id.Hex() != raw {
		t.Fatalf("full: %v %s", err, id.Hex())
	}
	spaced, err := parseIdentityHash("0123 4567 89ab cdef 0123 4567 89ab cdef")
	if err != nil || len(spaced) != 16 {
		t.Fatalf("spaced: %v %x", err, spaced)
	}
	if _, err := parseIdentityHash("ab"); err == nil {
		t.Fatal("short hash accepted")
	}
	if _, err := parseIdentityHash("zzzz"); err == nil {
		t.Fatal("non-hex accepted")
	}
	if _, err := parseFullID("01234567"); err == nil {
		t.Fatal("prefix accepted as full id")
	}
}

func TestUnit_IDHelpers(t *testing.T) {
	id := mustID(0x10)
	if len(id.Bytes()) != rrc.IdentityLength {
		t.Fatal("bytes")
	}
	if id.Prefix(12) != id.Hex()[:12] {
		t.Fatalf("prefix=%s", id.Prefix(12))
	}
	if id.Prefix(0) != id.Hex() || id.Prefix(99) != id.Hex() {
		t.Fatal("prefix bounds")
	}
	if _, ok := idFrom(nil); ok {
		t.Fatal("nil peer")
	}
	if _, ok := idFrom([]byte{1, 2}); ok {
		t.Fatal("short peer")
	}
}

func TestUnit_HasHexPrefix(t *testing.T) {
	if _, ok := hasHexPrefix("abc"); ok {
		t.Fatal("3 hex chars must not be a prefix")
	}
	p, ok := hasHexPrefix("abcdef")
	if !ok || len(p) != 3 {
		t.Fatalf("got %x ok=%v", p, ok)
	}
	if _, ok := hasHexPrefix("nick-name"); ok {
		t.Fatal("nick treated as hex")
	}
}

func TestUnit_KeyEqual(t *testing.T) {
	if !keyEqual("secret", "secret") {
		t.Fatal("equal keys")
	}
	if keyEqual("secret", "Secret") {
		t.Fatal("case must not match")
	}
	if keyEqual("ab", "abc") {
		t.Fatal("length mismatch must fail closed")
	}
	if keyEqual("", "x") || keyEqual("x", "") {
		t.Fatal("empty vs nonempty")
	}
	if !keyEqual("", "") {
		t.Fatal("empty equals empty")
	}
}

func TestUnit_TrustLists(t *testing.T) {
	tr := NewTrust()
	raw := "0123456789abcdef0123456789abcdef"
	if err := tr.Load([]string{raw}, []string{"ffffffffffffffffffffffffffffffff"}); err != nil {
		t.Fatal(err)
	}
	id, _ := parseFullID(raw)
	ban, _ := parseFullID("ffffffffffffffffffffffffffffffff")
	if !tr.IsTrusted(id) || !tr.IsBanned(ban) {
		t.Fatal("load")
	}
	tr.Ban(id)
	if !tr.IsBanned(id) {
		t.Fatal("ban")
	}
	hexes := tr.BannedHex()
	if len(hexes) != 2 {
		t.Fatalf("banned hex %v", hexes)
	}
	if !tr.Unban(id) || tr.IsBanned(id) {
		t.Fatal("unban")
	}
	trusted, banned := tr.Counts()
	if trusted != 1 || banned != 1 {
		t.Fatalf("counts %d %d", trusted, banned)
	}
	if err := tr.Load([]string{"nope"}, nil); err == nil {
		t.Fatal("bad trusted accepted")
	}
}

func TestUnit_StatsInc(t *testing.T) {
	s := NewStats()
	s.Inc("joins", 2)
	s.Inc("unknown", 9)
	s.Inc("bytes_in", 0)
	body := s.Format(&rrc.Hub{}, DefaultConfig(), NewTrust(), NewRoomRegistry("", 900))
	if !strings.Contains(body, "gorrcd") || !strings.Contains(body, "joins=2") {
		t.Fatalf("stats:\n%s", body)
	}
}

func TestUnit_AtomicWriteMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := atomicWrite(path, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", st.Mode().Perm())
	}
}

func TestUnit_FirstRunPermissions(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "gorrcd.toml")
	id := filepath.Join(dir, "hub_identity")
	rooms := filepath.Join(dir, "rooms.toml")
	created, err := FirstRun(cfg, id, rooms, filepath.Join(dir, "rns"))
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	for _, p := range []string{cfg, id, rooms} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode=%o", p, st.Mode().Perm())
		}
	}
	dst, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dst.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode=%o", dst.Mode().Perm())
	}
}

func TestUnit_NoticeChunks(t *testing.T) {
	s := NewService(testConfig(), nil)
	peer := mustPeer(1)
	s.notice(peer, "", "")
	s.notice(peer, "", strings.Repeat("x", maxNoticeChunk+40))
	s.errorTo(peer, "lobby", "not authorized")
	s.handleCommand(peer, "lobby", "/nope")
	s.handleCommand(peer, "", "/list")
	s.handleCommand(peer, "lobby", "/who")
	s.handleCommand(peer, "", "/stats")
}

func TestUnit_InterceptSlashOnly(t *testing.T) {
	s := NewService(testConfig(), nil)
	peer := mustPeer(2)
	env, err := rrc.NewEnvelope(rrc.TypeMsg, peer)
	if err != nil {
		t.Fatal(err)
	}
	env.Body = "/list"
	env.HasBody = true
	if !s.Intercept(peer, env) {
		t.Fatal("slash must intercept")
	}
	env.Body = "hello"
	if s.Intercept(peer, env) {
		t.Fatal("plain text must not intercept")
	}
	env.Body = "  /who  "
	if !s.Intercept(peer, env) {
		t.Fatal("padded slash must intercept")
	}
}

func TestUnit_OnIdentifiedBan(t *testing.T) {
	s := NewService(testConfig(), nil)
	id := mustID(0xaa)
	s.trust.Ban(id)
	if err := s.OnIdentified(id.Bytes()); err == nil || err.Error() != "banned" {
		t.Fatalf("err=%v", err)
	}
	if err := s.OnIdentified(mustPeer(0x11)); err != nil {
		t.Fatal(err)
	}
}

func TestUnit_AllowJoinDelegates(t *testing.T) {
	s := NewService(testConfig(), nil)
	founder := mustPeer(0x20)
	if err := s.AllowJoin(founder, "lobby", nil); err != nil {
		t.Fatal(err)
	}
	_ = s.rooms.SetFlag("lobby", "i", true, "")
	other := mustPeer(0x21)
	if err := s.AllowJoin(other, "lobby", nil); err == nil {
		t.Fatal("invite-only")
	}
}

func TestUnit_ModeStringOrder(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	f := mustID(1)
	reg.Ensure("x", f, true)
	_ = reg.SetFlag("x", "t", true, "")
	_ = reg.SetFlag("x", "m", true, "")
	_ = reg.SetFlag("x", "i", true, "")
	if got := reg.ModeString("x"); got != "+imt" {
		t.Fatalf("mode=%s", got)
	}
	if reg.ModeString("missing") != "(none)" {
		t.Fatal("missing")
	}
}

func TestUnit_RegisterSetsDefaults(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	f := mustID(3)
	if err := reg.Register("ops", f); err != nil {
		t.Fatal(err)
	}
	if !reg.Get("ops").Registered || !reg.Get("ops").NoOutsideMsgs || !reg.Get("ops").TopicOpsOnly {
		t.Fatalf("%+v", reg.Get("ops"))
	}
	if err := reg.Unregister("ops"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Unregister("ops"); err == nil {
		t.Fatal("second unregister")
	}
}
