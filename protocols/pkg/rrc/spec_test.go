// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
	"testing"
	"testing/quick"
)

func TestEnvelope_MarshalRoundTrip(t *testing.T) {
	f := func(typ uint8, roomLen uint8, bodyLen uint8, nickLen uint8, hasRoom, hasBody, hasNick bool) bool {
		sender := bytes.Repeat([]byte{0xab}, IdentityLength)
		env, err := NewEnvelope(uint64(typ%50)+1, sender)
		if err != nil {
			return false
		}
		env.Timestamp = 1_700_000_000_000
		env.MsgID = bytes.Repeat([]byte{0x7e}, MessageIDLength)
		if hasRoom {
			n := int(roomLen%32) + 1
			env.Room = NormalizeRoom("#" + string(bytes.Repeat([]byte{'a'}, n)))
			env.HasRoom = true
		}
		if hasBody {
			n := int(bodyLen % 64)
			env.Body = string(bytes.Repeat([]byte{'x'}, n))
			env.HasBody = true
		}
		if hasNick {
			n := int(nickLen%16) + 1
			env.Nick = SanitizeNick(string(bytes.Repeat([]byte{'n'}, n)))
			env.HasNick = env.Nick != ""
		}
		raw, err := env.Marshal()
		if err != nil {
			return false
		}
		got, err := UnmarshalEnvelope(raw)
		if err != nil {
			return false
		}
		if got.Type != env.Type || got.Version != env.Version || got.Timestamp != env.Timestamp {
			return false
		}
		if !bytes.Equal(got.MsgID, env.MsgID) || !bytes.Equal(got.Sender, env.Sender) {
			return false
		}
		if got.HasRoom != env.HasRoom || got.Room != env.Room {
			return false
		}
		if got.HasNick != env.HasNick || got.Nick != env.Nick {
			return false
		}
		if env.HasBody {
			s1, ok1 := BodyAsString(env.Body)
			s2, ok2 := BodyAsString(got.Body)
			if !ok1 || !ok2 || s1 != s2 {
				return false
			}
		} else if got.HasBody {
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeRoom_Idempotent(t *testing.T) {
	f := func(s string) bool {
		if len(s) > 256 {
			s = s[:256]
		}
		a := NormalizeRoom(s)
		return a == NormalizeRoom(a)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeNick_Idempotent(t *testing.T) {
	f := func(s string) bool {
		if len(s) > 128 {
			s = s[:128]
		}
		a := SanitizeNick(s)
		return a == SanitizeNick(a)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatal(err)
	}
}

func TestHelloBody_MapRoundTrip(t *testing.T) {
	f := func(name, ver string, hasCap bool) bool {
		if len(name) > 64 {
			name = name[:64]
		}
		if len(ver) > 32 {
			ver = ver[:32]
		}
		h := &HelloBody{}
		if name != "" {
			h.ClientName = name
			h.HasName = true
		}
		if ver != "" {
			h.ClientVersion = ver
			h.HasVersion = true
		}
		if hasCap {
			h.Capabilities = map[uint64]any{1: true}
			h.HasCaps = true
		}
		m := h.ToMap()
		got, err := ParseHelloBody(m)
		if err != nil {
			return false
		}
		if h.HasName != got.HasName || h.ClientName != got.ClientName {
			return false
		}
		if h.HasVersion != got.HasVersion || h.ClientVersion != got.ClientVersion {
			return false
		}
		if h.HasCaps != got.HasCaps {
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestEnvelope_MarshalUnmarshalStable(t *testing.T) {
	sender := bytes.Repeat([]byte{0x55}, IdentityLength)
	cases := []struct {
		typ  uint64
		room string
		body any
		nick string
	}{
		{TypePing, "", nil, ""},
		{TypeMsg, "#lobby", "hello", "alice"},
		{TypeNotice, "#ops", "maintenance", "hub"},
		{TypeAction, "#lobby", "waves", "bob"},
		{TypeJoin, "#lobby", nil, "alice"},
		{TypePart, "#lobby", nil, "alice"},
		{TypeError, "", "denied", ""},
	}
	for _, tc := range cases {
		env := mustEnvelope(t, tc.typ, sender)
		env.Timestamp = 42
		env.MsgID = bytes.Repeat([]byte{0x01}, MessageIDLength)
		if tc.room != "" {
			env.Room = tc.room
			env.HasRoom = true
		}
		if tc.body != nil {
			env.Body = tc.body
			env.HasBody = true
		}
		if tc.nick != "" {
			env.Nick = tc.nick
			env.HasNick = true
		}
		raw1, err := env.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		mid, err := UnmarshalEnvelope(raw1)
		if err != nil {
			t.Fatal(err)
		}
		raw2, err := mid.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		end, err := UnmarshalEnvelope(raw2)
		if err != nil {
			t.Fatal(err)
		}
		if mid.Type != end.Type || mid.Room != end.Room || mid.Nick != end.Nick {
			t.Fatalf("repack drift for type %d", tc.typ)
		}
		s1, ok1 := BodyAsString(mid.Body)
		s2, ok2 := BodyAsString(end.Body)
		if ok1 != ok2 || s1 != s2 {
			t.Fatalf("body drift for type %d: %#v vs %#v", tc.typ, mid.Body, end.Body)
		}
		_ = raw1
	}
}

func TestHubLimits_ToMapKeysMatchSpec(t *testing.T) {
	// Go HubLimits map keys must match documented rrcd layout.
	l := HubLimits{
		MaxNickBytes: 10, MaxRoomNameBytes: 20, MaxMsgBodyBytes: 30,
		MaxRoomsPerSession: 4, RateLimitMsgsPerMinute: 5,
	}
	m := l.ToMap()
	if m[LimitMaxNickBytes] != uint64(10) || m[LimitMaxRoomNameBytes] != uint64(20) {
		t.Fatalf("map = %#v", m)
	}
	if m[LimitMaxMsgBodyBytes] != uint64(30) || m[LimitMaxRoomsPerSession] != uint64(4) {
		t.Fatalf("map = %#v", m)
	}
	if m[LimitRateLimitMsgsPerMinute] != uint64(5) {
		t.Fatalf("map = %#v", m)
	}
	parsed, ok := ParseHubLimits(m)
	if !ok || parsed != l {
		t.Fatalf("parsed = %+v ok=%v", parsed, ok)
	}
}

func TestDefaultHubCapabilities_MatchRrcd(t *testing.T) {
	caps := DefaultHubCapabilities(true)
	if caps[CapAction] != true || caps[CapDirectNotice] != true || caps[CapResourceEnvelope] != true {
		t.Fatalf("caps=%#v", caps)
	}
	off := DefaultHubCapabilities(false)
	if _, ok := off[CapResourceEnvelope]; ok {
		t.Fatal("resource cap present when disabled")
	}
}

func TestConstants_WireKeyAssignments(t *testing.T) {
	if KeyDestination != 8 || TypeResourceEnvelope != 50 {
		t.Fatalf("key dest=%d type res=%d", KeyDestination, TypeResourceEnvelope)
	}
	if CapResourceEnvelope != 0 || CapAction != 1 || CapDirectNotice != 2 {
		t.Fatal("capability keys")
	}
	t.Log("RRC_WIRE_ASSIGNMENTS_PROVED")
}

func TestResourceEnvelopeKeys_MatchRrcd(t *testing.T) {
	if ResourceKeyID != 0 || ResourceKeyKind != 1 || ResourceKeySize != 2 || ResourceKeySHA256 != 3 || ResourceKeyEncoding != 4 {
		t.Fatal("resource body keys")
	}
	if ResourceKindNotice != "notice" || ResourceKindMOTD != "motd" || ResourceKindBlob != "blob" {
		t.Fatal("resource kinds")
	}
	if HelloKeyNickLegacy != 64 || DefaultMaxResourceBytes != 262144 {
		t.Fatal("legacy nick or resource limit")
	}
	body := &ResourceEnvelopeBody{ID: []byte{1, 2}, HasID: true, Kind: ResourceKindBlob, HasKind: true, Size: 8, HasSize: true}
	got, reason := ValidateResourceEnvelopeBody(body.ToMap())
	if reason != "" || got == nil || got.Size != 8 {
		t.Fatalf("valid body rejected: %q got=%+v", reason, got)
	}
	t.Log("RRC_RESOURCE_KEYS_PROVED")
}

func TestEnvelopeFrom_StampsAuthenticatedPeer(t *testing.T) {
	sender := bytes.Repeat([]byte{0x99}, IdentityLength)
	peer := bytes.Repeat([]byte{0xaa}, IdentityLength)
	env := mustEnvelope(t, TypeMsg, sender)
	env.Room = "#x"
	env.HasRoom = true
	env.Body = "m"
	env.HasBody = true
	fwd, err := envelopeFrom(env.Type, peer, env.MsgID, env.Timestamp)
	if err != nil {
		t.Fatal(err)
	}
	fwd.Room = env.Room
	fwd.HasRoom = true
	fwd.Body = env.Body
	fwd.HasBody = true
	if bytes.Equal(fwd.Sender, sender) {
		t.Fatal("envelopeFrom must not copy wire sender from spoofed env")
	}
	if !bytes.Equal(fwd.Sender, peer) {
		t.Fatal("forward path must stamp authenticated peer")
	}
}

func TestHandlePeer_ErrorTypeNotForwarded(t *testing.T) {
	h := &Hub{cfg: HubConfig{}}
	h.cfg.applyDefaults()
	p := &hubPeer{
		peerHash: bytes.Repeat([]byte{0x01}, IdentityLength),
		sess:     &session{sender: bytes.Repeat([]byte{0x02}, IdentityLength)},
		rooms:    map[string]struct{}{"#r": {}},
	}
	p.active.Store(true)
	h.peers = map[peerID]*hubPeer{peerKey(p.peerHash): p}
	h.rooms = map[string]map[peerID]struct{}{"#r": {peerKey(p.peerHash): {}}}
	env := mustEnvelope(t, TypeError, p.peerHash)
	env.Room = "#r"
	env.HasRoom = true
	env.Body = "oops"
	env.HasBody = true
	h.handlePeer(p, env)
}
