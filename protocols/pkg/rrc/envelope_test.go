// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
	"strings"
	"testing"
)

func TestNormalizeRoom(t *testing.T) {
	if got := NormalizeRoom("  #Lobby "); got != "#lobby" {
		t.Fatalf("NormalizeRoom = %q", got)
	}
}

func TestSanitizeNick(t *testing.T) {
	if got := SanitizeNick("  alice\n"); got != "alice" {
		t.Fatalf("SanitizeNick = %q", got)
	}
}

func TestEnvelopeRoundTripMSG(t *testing.T) {
	sender := bytes.Repeat([]byte{0x9c}, IdentityLength)
	env, err := NewEnvelope(TypeMsg, sender)
	if err != nil {
		t.Fatal(err)
	}
	env.Room = "#lobby"
	env.HasRoom = true
	env.Body = "Hello, world!"
	env.HasBody = true
	env.Nick = "alice"
	env.HasNick = true
	env.Timestamp = 1737849600000
	env.MsgID = []byte{0x7a, 0x3f, 0x8e, 0x12, 0x45, 0xc9, 0xa1, 0x6d}

	raw, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < FixedEnvelopeMin {
		t.Fatalf("encoded len %d < fixed min %d", len(raw), FixedEnvelopeMin)
	}

	got, err := UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != TypeMsg || got.Version != ProtocolVersion {
		t.Fatalf("type/version = %d/%d", got.Type, got.Version)
	}
	if !bytes.Equal(got.Sender, sender) {
		t.Fatal("sender mismatch")
	}
	if got.Room != "#lobby" || !got.HasRoom {
		t.Fatalf("room = %q has=%v", got.Room, got.HasRoom)
	}
	s, ok := BodyAsString(got.Body)
	if !ok || s != "Hello, world!" {
		t.Fatalf("body = %#v", got.Body)
	}
	if got.Nick != "alice" || !got.HasNick {
		t.Fatalf("nick = %q", got.Nick)
	}
}

func TestEnvelopeDestinationRoundTrip(t *testing.T) {
	sender := bytes.Repeat([]byte{0x11}, IdentityLength)
	dst := bytes.Repeat([]byte{0x22}, IdentityLength)
	env, err := NewEnvelope(TypeNotice, sender)
	if err != nil {
		t.Fatal(err)
	}
	env.Destination = dst
	env.HasDestination = true
	env.Body = "dm"
	env.HasBody = true
	raw, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasDestination || !bytes.Equal(got.Destination, dst) {
		t.Fatalf("dest=%x has=%v", got.Destination, got.HasDestination)
	}
}

func TestEnvelopeIgnoresUnknownKeys(t *testing.T) {
	sender := bytes.Repeat([]byte{0x11}, IdentityLength)
	env, err := NewEnvelope(TypePing, sender)
	if err != nil {
		t.Fatal(err)
	}
	raw2, err := mustMarshalWithExtra(*env, 99, "future")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := UnmarshalEnvelope(raw2)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Type != TypePing {
		t.Fatalf("type = %d", parsed.Type)
	}
}

func TestEnvelopeWrongVersion(t *testing.T) {
	sender := bytes.Repeat([]byte{0x22}, IdentityLength)
	env, err := NewEnvelope(TypeHello, sender)
	if err != nil {
		t.Fatal(err)
	}
	env.Version = 99
	_, err = env.Marshal()
	if err != ErrWrongVersion {
		t.Fatalf("marshal err = %v", err)
	}
	env.Version = ProtocolVersion
	raw, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	// Tamper version in map
	raw2, err := mustMarshalWithVersion(env, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = UnmarshalEnvelope(raw2)
	if err != ErrWrongVersion {
		t.Fatalf("unmarshal err = %v", err)
	}
	_ = raw
}

func TestEnvelopeMissingFields(t *testing.T) {
	_, err := UnmarshalEnvelope(nil)
	if err != ErrInvalidEnvelope {
		t.Fatalf("nil data err = %v", err)
	}
}

func TestHelloWelcomeBodies(t *testing.T) {
	h := &HelloBody{ClientName: "go-rrc", HasName: true, ClientVersion: "0.1.0", HasVersion: true}
	sender := bytes.Repeat([]byte{0x33}, IdentityLength)
	env, err := NewEnvelope(TypeHello, sender)
	if err != nil {
		t.Fatal(err)
	}
	env.Body = h.ToMap()
	env.HasBody = true
	raw, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := ParseHelloBody(got.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !hb.HasName || hb.ClientName != "go-rrc" {
		t.Fatalf("hello body = %+v", hb)
	}

	w := &WelcomeBody{
		HubName: "ExampleHub", HasName: true,
		HubVersion: "0.1.0", HasVersion: true,
		Limits: HubLimits{
			MaxNickBytes: 32, MaxRoomsPerSession: 32,
			MaxRoomNameBytes: 64, MaxMsgBodyBytes: 350,
			RateLimitMsgsPerMinute: 60,
		},
		HasLimits: true,
	}
	env2, err := NewEnvelope(TypeWelcome, sender)
	if err != nil {
		t.Fatal(err)
	}
	env2.Body = w.ToMap()
	env2.HasBody = true
	raw2, err := env2.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got2, err := UnmarshalEnvelope(raw2)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := ParseWelcomeBody(got2.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !wb.HasName || wb.HubName != "ExampleHub" || !wb.HasLimits || wb.Limits.MaxMsgBodyBytes != 350 {
		t.Fatalf("welcome body = %+v", wb)
	}
}

func TestWelcomeCapabilitiesOnWire(t *testing.T) {
	sender := bytes.Repeat([]byte{0x33}, IdentityLength)
	w := &WelcomeBody{
		HubName: "cap-hub", HasName: true,
		HubVersion: "0.1.0", HasVersion: true,
		Capabilities: DefaultHubCapabilities(false),
		HasCaps:      true,
		Limits: HubLimits{
			MaxNickBytes: 4, MaxRoomsPerSession: 32,
			MaxRoomNameBytes: 64, MaxMsgBodyBytes: 8,
			RateLimitMsgsPerMinute: 60,
		},
		HasLimits: true,
	}
	env, err := NewEnvelope(TypeWelcome, sender)
	if err != nil {
		t.Fatal(err)
	}
	env.Body = w.ToMap()
	env.HasBody = true
	raw, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatalf("unmarshal welcome: %v", err)
	}
	parsed, err := ParseWelcomeBody(got.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.HasCaps || parsed.Capabilities[CapAction] != true {
		t.Fatalf("caps=%+v", parsed)
	}
}

func TestJoinedMembers(t *testing.T) {
	a := bytes.Repeat([]byte{0x01}, IdentityLength)
	b := bytes.Repeat([]byte{0x02}, IdentityLength)
	members, err := ParseJoinedMembers([]any{a, b, "skip"})
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("len = %d", len(members))
	}
}

func TestResourceEnvelopeBodyRoundTrip(t *testing.T) {
	id := []byte{1, 2, 3, 4}
	hash := bytes.Repeat([]byte{0xab}, 32)
	r := &ResourceEnvelopeBody{
		ID: id, HasID: true,
		Kind: ResourceKindNotice, HasKind: true,
		Size: 99, HasSize: true,
		SHA256: hash, HasHash: true,
		Encoding: "utf-8", HasEnc: true,
	}
	got, err := ParseResourceEnvelopeBody(r.ToMap())
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasID || !bytes.Equal(got.ID, id) || got.Kind != ResourceKindNotice || got.Size != 99 {
		t.Fatalf("got=%+v", got)
	}
	if !got.HasHash || !bytes.Equal(got.SHA256, hash) || got.Encoding != "utf-8" {
		t.Fatalf("hash/enc %+v", got)
	}
}

func TestHelloLegacyNick(t *testing.T) {
	if HelloLegacyNick(nil) != "" {
		t.Fatal("nil")
	}
	m := map[uint64]any{HelloKeyNickLegacy: "old-nick"}
	if HelloLegacyNick(m) != "old-nick" {
		t.Fatalf("got %q", HelloLegacyNick(m))
	}
}

func TestBodySizeBytes(t *testing.T) {
	if BodySizeBytes("café") != uint64(len("café")) {
		t.Fatalf("utf8 size %d", BodySizeBytes("café"))
	}
	if BodySizeBytes([]byte{1, 2, 3}) != 3 {
		t.Fatal("bytes")
	}
	if BodySizeBytes(nil) != 0 {
		t.Fatal("nil")
	}
	if BodySizeBytes(map[uint64]any{0: strings.Repeat("x", 40)}) <= 8 {
		t.Fatal("structured size should exceed key overhead")
	}
	if sizeAsUint64(-1) != ^uint64(0) {
		t.Fatal("negative size must be treated as oversized")
	}
}

func TestUnmarshalEnvelopeRejectsOversize(t *testing.T) {
	data := make([]byte, MaxEnvelopeBytes+1)
	_, err := UnmarshalEnvelope(data)
	if err != ErrEnvelopeTooLarge {
		t.Fatalf("err = %v", err)
	}
}

func TestUnmarshalEnvelopeRejectsDeepNesting(t *testing.T) {
	inner := []byte{0xa0}
	for range 16 {
		wrapped := make([]byte, 0, 2+len(inner))
		wrapped = append(wrapped, 0xa1, 0x00)
		wrapped = append(wrapped, inner...)
		inner = wrapped
	}
	_, err := UnmarshalEnvelope(inner)
	if err == nil {
		t.Fatal("expected nested map to be rejected")
	}
}

func TestEnvelopeFromPreservesMsgID(t *testing.T) {
	sender := bytes.Repeat([]byte{0x11}, IdentityLength)
	msgID := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	env, err := envelopeFrom(TypeMsg, sender, msgID, 99)
	if err != nil {
		t.Fatal(err)
	}
	if env.Timestamp != 99 || !bytes.Equal(env.MsgID, msgID) {
		t.Fatalf("ts=%d id=%x", env.Timestamp, env.MsgID)
	}
	msgID[0] = 0xff
	if env.MsgID[0] == 0xff {
		t.Fatal("envelopeFrom aliased msgid")
	}
}

func TestValidateResourceEnvelopeBody(t *testing.T) {
	ok := map[uint64]any{
		ResourceKeyID:   []byte{1, 2, 3},
		ResourceKeyKind: ResourceKindNotice,
		ResourceKeySize: uint64(12),
	}
	if _, reason := ValidateResourceEnvelopeBody(ok); reason != "" {
		t.Fatalf("valid: %q", reason)
	}
	if _, reason := ValidateResourceEnvelopeBody("nope"); reason != "invalid resource envelope body" {
		t.Fatalf("non-map: %q", reason)
	}
	if _, reason := ValidateResourceEnvelopeBody(map[uint64]any{ResourceKeyKind: "blob", ResourceKeySize: uint64(1)}); !strings.Contains(reason, "id") {
		t.Fatalf("missing id: %q", reason)
	}
	if _, reason := ValidateResourceEnvelopeBody(map[uint64]any{ResourceKeyID: []byte{1}, ResourceKeySize: uint64(1)}); !strings.Contains(reason, "kind") {
		t.Fatalf("missing kind: %q", reason)
	}
	if _, reason := ValidateResourceEnvelopeBody(map[uint64]any{ResourceKeyID: []byte{1}, ResourceKeyKind: "", ResourceKeySize: uint64(1)}); !strings.Contains(reason, "kind") {
		t.Fatalf("empty kind: %q", reason)
	}
	if _, reason := ValidateResourceEnvelopeBody(map[uint64]any{ResourceKeyID: []byte{1}, ResourceKeyKind: "blob"}); !strings.Contains(reason, "size") {
		t.Fatalf("missing size: %q", reason)
	}
	if _, reason := ValidateResourceEnvelopeBody(map[uint64]any{ResourceKeyID: []byte{1}, ResourceKeyKind: "blob", ResourceKeySize: uint64(1), ResourceKeySHA256: "nope"}); !strings.Contains(reason, "sha256") {
		t.Fatalf("bad sha256: %q", reason)
	}
}

func TestMutation_ValidateResourceKillsBadBodies(t *testing.T) {
	valid := map[uint64]any{
		ResourceKeyID:   []byte{1, 2, 3, 4},
		ResourceKeyKind: ResourceKindBlob,
		ResourceKeySize: uint64(32),
	}
	if _, reason := ValidateResourceEnvelopeBody(valid); reason != "" {
		t.Fatalf("baseline valid rejected: %q", reason)
	}
	mutants := []struct {
		name string
		body any
		want string
	}{
		{"not-map", "x", "invalid resource envelope body"},
		{"drop-id", map[uint64]any{ResourceKeyKind: ResourceKindBlob, ResourceKeySize: uint64(1)}, "id"},
		{"drop-kind", map[uint64]any{ResourceKeyID: []byte{1}, ResourceKeySize: uint64(1)}, "kind"},
		{"empty-kind", map[uint64]any{ResourceKeyID: []byte{1}, ResourceKeyKind: "", ResourceKeySize: uint64(1)}, "kind"},
		{"drop-size", map[uint64]any{ResourceKeyID: []byte{1}, ResourceKeyKind: ResourceKindBlob}, "size"},
		{"bad-sha", map[uint64]any{ResourceKeyID: []byte{1}, ResourceKeyKind: ResourceKindBlob, ResourceKeySize: uint64(1), ResourceKeySHA256: 3}, "sha256"},
	}
	for _, m := range mutants {
		_, reason := ValidateResourceEnvelopeBody(m.body)
		if reason == "" || !strings.Contains(reason, m.want) {
			t.Fatalf("%s survived: reason=%q", m.name, reason)
		}
	}
	t.Log("RRC_RESOURCE_VALIDATE_PROVED")
}
