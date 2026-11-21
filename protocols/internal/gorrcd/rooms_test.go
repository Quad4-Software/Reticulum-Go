// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"encoding/hex"
	"strings"
	"testing"
	"testing/quick"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func TestParseFullID_RoundTrip(t *testing.T) {
	f := func(b0, b1, b2, b3 byte) bool {
		raw := make([]byte, rrc.IdentityLength)
		raw[0], raw[1], raw[2], raw[3] = b0, b1, b2, b3
		for i := 4; i < len(raw); i++ {
			raw[i] = byte(i) ^ b0
		}
		id, err := parseFullID(hex.EncodeToString(raw))
		if err != nil {
			return false
		}
		return hex.EncodeToString(id.Bytes()) == hex.EncodeToString(raw)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
	t.Log("GORRCD_PARSE_FULL_ID_PROVED")
}

func TestSplitCmd_FirstToken(t *testing.T) {
	f := func(cmd string) bool {
		if len(cmd) > 64 {
			cmd = cmd[:64]
		}
		cmd = strings.Map(func(r rune) rune {
			if r == 0 || r == '\n' || r == '\r' {
				return -1
			}
			return r
		}, cmd)
		got := splitCmd("/" + strings.TrimSpace(cmd))
		fields := strings.Fields(strings.TrimSpace(cmd))
		if len(fields) == 0 {
			return len(got) == 0
		}
		return len(got) > 0 && got[0] == fields[0]
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
	t.Log("GORRCD_SPLIT_CMD_PROVED")
}

func TestKeyEqual_RequiresEqualLength(t *testing.T) {
	f := func(a, b string) bool {
		if len(a) > 64 {
			a = a[:64]
		}
		if len(b) > 64 {
			b = b[:64]
		}
		eq := keyEqual(a, b)
		if len(a) != len(b) {
			return !eq
		}
		return eq == (a == b)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatal(err)
	}
	t.Log("GORRCD_KEY_EQUAL_PROVED")
}

func TestRoomRegistry_AllowJoinErrorStrings(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	founder := mustID(1)
	other := mustID(2)
	reg.Ensure("lobby", founder, true)
	_ = reg.SetFlag("lobby", "i", true, "")
	if err := reg.AllowJoin("lobby", other, nil, false); err == nil || err.Error() != "invite-only (+i)" {
		t.Fatalf("invite: %v", err)
	}
	_ = reg.SetFlag("lobby", "i", false, "")
	_ = reg.SetFlag("lobby", "k", true, "secret")
	if err := reg.AllowJoin("lobby", other, "nope", false); err == nil || err.Error() != "bad key (+k)" {
		t.Fatalf("key: %v", err)
	}
	_ = reg.AddBan("lobby", other)
	if err := reg.AllowJoin("lobby", other, "secret", false); err == nil || err.Error() != "banned from room" {
		t.Fatalf("ban: %v", err)
	}
	t.Log("GORRCD_RRCD_JOIN_ERRORS_PROVED")
}

func TestRoomRegistry_AllowContentErrorStrings(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	founder := mustID(1)
	other := mustID(2)
	reg.Ensure("mod", founder, true)
	_ = reg.SetFlag("mod", "n", true, "")
	if err := reg.AllowContent("mod", other, false, false); err == nil || err.Error() != "no outside messages (+n)" {
		t.Fatalf("+n: %v", err)
	}
	_ = reg.SetFlag("mod", "n", false, "")
	_ = reg.SetFlag("mod", "m", true, "")
	if err := reg.AllowContent("mod", other, true, false); err == nil || err.Error() != "room is moderated (+m)" {
		t.Fatalf("+m: %v", err)
	}
	t.Log("GORRCD_RRCD_CONTENT_ERRORS_PROVED")
}

func TestRoomRegistry_FounderRemainsOpAfterDelOp(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	founder := mustID(9)
	reg.Ensure("ops", founder, true)
	if !reg.IsOp("ops", founder, false) {
		t.Fatal("founder starts as op")
	}
	_ = reg.DelOp("ops", founder)
	if !reg.IsOp("ops", founder, false) {
		t.Fatal("founder must remain op after DelOp")
	}
	t.Log("GORRCD_FOUNDER_OP_PROVED")
}

func TestRoomRegistry_TrustedJoinBypassesInviteAndKey(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	founder := mustID(1)
	trusted := mustID(3)
	reg.Ensure("vip", founder, true)
	_ = reg.SetFlag("vip", "i", true, "")
	_ = reg.SetFlag("vip", "k", true, "secret")
	if err := reg.AllowJoin("vip", trusted, nil, true); err != nil {
		t.Fatal(err)
	}
	t.Log("GORRCD_TRUSTED_BYPASS_PROVED")
}
