// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestUnit_NewEnvelopeRejectsBadSender(t *testing.T) {
	_, err := NewEnvelope(TypeMsg, []byte{0x01})
	if !errors.Is(err, ErrBadFieldLength) {
		t.Fatalf("err = %v", err)
	}
}

func TestUnit_MarshalNilEnvelope(t *testing.T) {
	var e *Envelope
	_, err := e.Marshal()
	if !errors.Is(err, ErrNilArgument) {
		t.Fatalf("err = %v", err)
	}
}

func TestUnit_MarshalBadMsgID(t *testing.T) {
	sender := bytes.Repeat([]byte{0x22}, IdentityLength)
	env := mustEnvelope(t, TypePing, sender)
	env.MsgID = []byte{1, 2}
	_, err := env.Marshal()
	if !errors.Is(err, ErrBadFieldLength) {
		t.Fatalf("err = %v", err)
	}
}

func TestUnit_UnmarshalCorruptCBOR(t *testing.T) {
	_, err := UnmarshalEnvelope([]byte{0xff, 0x00, 0x01})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnit_UnmarshalMissingType(t *testing.T) {
	m := map[uint64]any{
		KeyVersion:   ProtocolVersion,
		KeyMsgID:     bytes.Repeat([]byte{0x01}, MessageIDLength),
		KeyTimestamp: uint64(1),
		KeySender:    bytes.Repeat([]byte{0x02}, IdentityLength),
	}
	raw, err := mustMarshalMap(m)
	if err != nil {
		t.Fatal(err)
	}
	_, err = UnmarshalEnvelope(raw)
	if err == nil || !strings.Contains(err.Error(), "type") {
		t.Fatalf("err = %v", err)
	}
}

func TestUnit_BodyAsString(t *testing.T) {
	if s, ok := BodyAsString("hi"); !ok || s != "hi" {
		t.Fatalf("got %q %v", s, ok)
	}
	if _, ok := BodyAsString(123); ok {
		t.Fatal("expected false")
	}
}

func TestUnit_ParseHelloBodyEmpty(t *testing.T) {
	h, err := ParseHelloBody(nil)
	if err != nil || h == nil {
		t.Fatalf("err=%v h=%v", err, h)
	}
}

func TestUnit_ParseHelloBodyInvalid(t *testing.T) {
	_, err := ParseHelloBody("not-a-map")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnit_ParseJoinedMembersSkipsBad(t *testing.T) {
	good := bytes.Repeat([]byte{0x09}, IdentityLength)
	out, err := ParseJoinedMembers([]any{good, []byte{1}, 7, nil})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("len=%d", len(out))
	}
}

func TestUnit_HelloWelcomeToMapOmitsEmpty(t *testing.T) {
	if (&HelloBody{}).ToMap() != nil {
		t.Fatal("expected nil map")
	}
	if (&WelcomeBody{}).ToMap() != nil {
		t.Fatal("expected nil map")
	}
}

func TestUnit_SanitizeNickStripsControls(t *testing.T) {
	got := SanitizeNick("a\x00b\nc")
	if got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestUnit_NormalizeRoomCaseFold(t *testing.T) {
	if NormalizeRoom("#GENERAL") != NormalizeRoom("#general") {
		t.Fatal("case fold mismatch")
	}
}

func TestUnit_FormatError(t *testing.T) {
	if FormatError(nil) != "" {
		t.Fatal("nil")
	}
	env := &Envelope{Body: "boom", HasBody: true}
	if FormatError(env) != "boom" {
		t.Fatalf("got %q", FormatError(env))
	}
}

func TestUnit_NewHubNilArgs(t *testing.T) {
	if _, err := NewHub(nil, nil, HubConfig{}); !errors.Is(err, ErrNilArgument) {
		t.Fatalf("err=%v", err)
	}
	if _, err := NewHubDestination(nil, nil); !errors.Is(err, ErrNilArgument) {
		t.Fatalf("err=%v", err)
	}
}

func TestUnit_DialNilArgs(t *testing.T) {
	if _, err := Dial(nil, nil, nil, ClientConfig{}); !errors.Is(err, ErrNilArgument) {
		t.Fatalf("err=%v", err)
	}
}

func TestUnit_ClientCloseIdempotent(t *testing.T) {
	c := &Client{state: ClientDisconnected, rooms: map[string]struct{}{}}
	c.Close()
	c.Close()
}

func TestUnit_HubRoomMembersEmpty(t *testing.T) {
	h := &Hub{rooms: map[string]map[peerID]struct{}{}, peers: map[peerID]*hubPeer{}}
	if len(h.RoomMembers("#x")) != 0 {
		t.Fatal("expected empty")
	}
}

func TestUnit_UnknownMessageTypesIgnoredConstants(t *testing.T) {
	// Spec reserves 0 and allows ignoring unknown types. Ensure our assignments stay stable.
	seen := map[uint64]string{
		TypeHello: "HELLO", TypeWelcome: "WELCOME",
		TypeJoin: "JOIN", TypeJoined: "JOINED", TypePart: "PART", TypeParted: "PARTED",
		TypeMsg: "MSG", TypeNotice: "NOTICE", TypeAction: "ACTION",
		TypePing: "PING", TypePong: "PONG", TypeError: "ERROR",
		TypeResourceEnvelope: "RESOURCE_ENVELOPE",
	}
	if len(seen) != 13 {
		t.Fatal("type table size")
	}
}

func TestParseHashGrouped(t *testing.T) {
	raw := bytes.Repeat([]byte{0xab}, IdentityLength)
	got, err := ParseHash(" <" + FormatHash(raw) + "> ")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("got %x", got)
	}
}

func TestParseHashRejectsEmpty(t *testing.T) {
	if _, err := ParseHash(" <> "); !errors.Is(err, ErrInvalidHash) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseWelcomeBodyRejectsNonMap(t *testing.T) {
	_, err := ParseWelcomeBody("nope")
	if !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("err = %v", err)
	}
}

func TestDialHashRejectsEmpty(t *testing.T) {
	_, err := DialHash(nil, nil, "", ClientConfig{})
	if !errors.Is(err, ErrInvalidHash) {
		t.Fatalf("err = %v", err)
	}
}
