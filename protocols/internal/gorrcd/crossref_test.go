// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"testing"
)

func TestCrossRef_RrcdErrorStrings(t *testing.T) {
	if rrcdInviteOnly != "invite-only (+i)" {
		t.Fatal(rrcdInviteOnly)
	}
	if rrcdBadKey != "bad key (+k)" {
		t.Fatal(rrcdBadKey)
	}
	if rrcdBannedFromRoom != "banned from room" {
		t.Fatal(rrcdBannedFromRoom)
	}
	if rrcdNoOutside != "no outside messages (+n)" {
		t.Fatal(rrcdNoOutside)
	}
	if rrcdModerated != "room is moderated (+m)" {
		t.Fatal(rrcdModerated)
	}
	if rrcdNotAuthorized != "not authorized" {
		t.Fatal(rrcdNotAuthorized)
	}
	if rrcdNotAuthorizedTopic != "not authorized (+t)" {
		t.Fatal(rrcdNotAuthorizedTopic)
	}
	if rrcdUnrecognized != "unrecognized command" {
		t.Fatal(rrcdUnrecognized)
	}
	if rrcdBanned != "banned" {
		t.Fatal(rrcdBanned)
	}

	reg := NewRoomRegistry("", 900)
	founder := mustID(1)
	other := mustID(2)
	reg.Ensure("x", founder, true)
	_ = reg.SetFlag("x", "i", true, "")
	if err := reg.AllowJoin("x", other, nil, false); err == nil || err.Error() != rrcdInviteOnly {
		t.Fatalf("invite: %v", err)
	}
	_ = reg.SetFlag("x", "i", false, "")
	_ = reg.SetFlag("x", "k", true, "k")
	if err := reg.AllowJoin("x", other, "no", false); err == nil || err.Error() != rrcdBadKey {
		t.Fatalf("key: %v", err)
	}
	_ = reg.AddBan("x", other)
	if err := reg.AllowJoin("x", other, "k", false); err == nil || err.Error() != rrcdBannedFromRoom {
		t.Fatalf("ban join: %v", err)
	}
	_ = reg.DelBan("x", other)
	_ = reg.SetFlag("x", "k", false, "")
	_ = reg.SetFlag("x", "n", true, "")
	if err := reg.AllowContent("x", other, false, false); err == nil || err.Error() != rrcdNoOutside {
		t.Fatalf("+n: %v", err)
	}
	_ = reg.SetFlag("x", "n", false, "")
	_ = reg.SetFlag("x", "m", true, "")
	if err := reg.AllowContent("x", other, true, false); err == nil || err.Error() != rrcdModerated {
		t.Fatalf("+m: %v", err)
	}

	s := NewService(testConfig(), nil)
	s.trust.Ban(other)
	if err := s.OnIdentified(other.Bytes()); err == nil || err.Error() != rrcdBanned {
		t.Fatalf("kline: %v", err)
	}
	t.Log("GORRCD_RRCD_CROSSREF_PROVED")
}

func TestCrossRef_ModeFlagLetters(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	f := mustID(1)
	reg.Ensure("m", f, true)
	for _, flag := range []string{"i", "k", "m", "n", "p", "t"} {
		key := ""
		if flag == "k" {
			key = "x"
		}
		if err := reg.SetFlag("m", flag, true, key); err != nil {
			t.Fatalf("flag %s: %v", flag, err)
		}
	}
	mode := reg.ModeString("m")
	for _, c := range []byte("ikmnpt") {
		if !containsByte(mode, c) {
			t.Fatalf("mode %s missing %c", mode, c)
		}
	}
}

func containsByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}
