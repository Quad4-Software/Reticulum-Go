// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

func TestLXMessage_StringAndAccessors(t *testing.T) {
	m := &LXMessage{}
	if m.String() != "<LXMessage>" {
		t.Errorf("expected sentinel string, got %s", m.String())
	}
	m.Hash = make([]byte, 32)
	if got := m.String(); !strings.HasPrefix(got, "<LXMessage ") {
		t.Errorf("expected hash representation, got %s", got)
	}
	if m.FormatHash() == "" {
		t.Error("FormatHash should return hex of hash")
	}

	m.SetTitle("subj")
	m.SetContent("body")
	if m.TitleString() != "subj" || m.ContentString() != "body" {
		t.Error("SetTitle/SetContent did not update message")
	}
}

func TestEncryptedPayload_RequiresPack(t *testing.T) {
	m := &LXMessage{}
	if _, err := m.EncryptedPayload(); err == nil {
		t.Fatal("expected error for unpacked message")
	}
}

func TestAsByteKey_OutOfRange(t *testing.T) {
	cases := []any{int8(-1), int16(-1), int16(0x100), int32(-1), int32(0x100), int64(-1), int64(0x100), uint16(0x100), uint32(0x100), uint64(0x100)}
	for _, c := range cases {
		if _, err := asByteKey(c); !errors.Is(err, ErrInvalidPayload) {
			t.Errorf("expected ErrInvalidPayload for %#v, got %v", c, err)
		}
	}
	if _, err := asByteKey(struct{}{}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("expected ErrInvalidPayload for unsupported type, got %v", err)
	}

	if v, err := asByteKey(uint8(0xAB)); err != nil || v != 0xAB {
		t.Errorf("uint8 keys should round-trip, got %v err %v", v, err)
	}
	if v, err := asByteKey(int64(0x42)); err != nil || v != 0x42 {
		t.Errorf("int64 valid key failed: %v err %v", v, err)
	}
}

func TestApplyPayload_RejectsBadShape(t *testing.T) {
	m := &LXMessage{}
	if err := m.applyPayload([]any{1.0, "a", "b"}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("expected ErrInvalidPayload for short payload, got %v", err)
	}

	if err := m.applyPayload([]any{"not-a-number", []byte("t"), []byte("c"), nil}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("expected ErrInvalidPayload for bad timestamp, got %v", err)
	}
}

func TestAsFields_StringKeyMap(t *testing.T) {
	got, err := asFields(map[string]any{string([]byte{0x0F}): []byte{0x01}})
	if err != nil {
		t.Fatalf("asFields: %v", err)
	}
	if got[FieldRenderer] == nil {
		t.Errorf("expected key 0x0F populated")
	}

	if _, err := asFields(map[string]any{"longkey": nil}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("expected ErrInvalidPayload for multi-byte key, got %v", err)
	}
}

func TestAsFields_NormalizesNestedDicts(t *testing.T) {
	raw := map[any]any{
		int64(FieldReaction): map[any]any{
			int64(ReactionTo):      []byte{1, 2},
			int64(ReactionContent): []byte("ok"),
		},
	}
	got, err := asFields(raw)
	if err != nil {
		t.Fatalf("asFields: %v", err)
	}
	reaction, ok := got[FieldReaction].(map[byte]any)
	if !ok {
		t.Fatalf("reaction field type %T", got[FieldReaction])
	}
	if reaction[ReactionTo] == nil || reaction[ReactionContent] == nil {
		t.Fatalf("nested keys not normalized: %#v", reaction)
	}
}

func TestStampCostFromAppData_VariousIntegerWidths(t *testing.T) {
	for _, cost := range []int64{0, 1, 64, 0xFFFF, 1 << 30} {
		blob, err := EncodeAnnounceAppDataV5("peer", cost)
		if err != nil {
			t.Fatalf("encode v5: %v", err)
		}
		got, ok, err := StampCostFromAppData(blob)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !ok || got != cost {
			t.Errorf("want %d, got %d ok=%v", cost, got, ok)
		}
	}
}

func TestMessageID_AliasesHash(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	msg, err := NewMessage(dst.Hash(), src.Hash(), nil, nil, nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := msg.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if !bytes.Equal(msg.MessageID(), msg.Hash) {
		t.Error("MessageID must alias Hash")
	}
}

func TestPackedSize_AndContentSize(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	content := []byte("size-test-content")
	msg, err := NewMessage(dst.Hash(), src.Hash(), []byte("t"), content, nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := msg.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if msg.PackedSize() != len(msg.Packed) {
		t.Errorf("PackedSize mismatch")
	}
	cs, err := msg.ContentSize()
	if err != nil {
		t.Fatalf("ContentSize: %v", err)
	}
	if cs <= 0 || cs > msg.PackedSize() {
		t.Errorf("unexpected content size %d (packed=%d)", cs, msg.PackedSize())
	}
}

func TestContentSize_RequiresPack(t *testing.T) {
	if _, err := (&LXMessage{}).ContentSize(); err == nil {
		t.Fatal("expected error for unpacked message")
	}
}

func TestChooseDeliveryMethod_FallsBackToDirect(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	huge := bytes.Repeat([]byte("X"), EncryptedPacketMaxContent+50)
	msg, err := NewMessage(dst.Hash(), src.Hash(), nil, huge, nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := msg.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	method, repr, err := msg.ChooseDeliveryMethod(MethodOpportunistic, DestinationTypeSingle)
	if err != nil {
		t.Fatalf("ChooseDeliveryMethod: %v", err)
	}
	if method != MethodDirect {
		t.Errorf("expected fallback to direct, got %d", method)
	}
	if repr != RepresentationPacket {
		t.Errorf("expected packet representation, got %d", repr)
	}
}

func TestChooseDeliveryMethod_DirectResource(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	huge := bytes.Repeat([]byte("X"), LinkPacketMaxContent+50)
	msg, err := NewMessage(dst.Hash(), src.Hash(), nil, huge, nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := msg.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	method, repr, err := msg.ChooseDeliveryMethod(MethodDirect, DestinationTypeSingle)
	if err != nil {
		t.Fatalf("ChooseDeliveryMethod: %v", err)
	}
	if method != MethodDirect || repr != RepresentationResource {
		t.Errorf("expected direct/resource, got %d/%d", method, repr)
	}
}

func TestChooseDeliveryMethod_PaperRespectsMDU(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	tiny, err := NewMessage(dst.Hash(), src.Hash(), nil, []byte("x"), nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := tiny.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if _, _, err := tiny.ChooseDeliveryMethod(MethodPaper, DestinationTypeSingle); err != nil {
		t.Fatalf("paper for tiny payload should succeed: %v", err)
	}

	huge := bytes.Repeat([]byte("X"), PaperMDU+1)
	bigMsg, err := NewMessage(dst.Hash(), src.Hash(), nil, huge, nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := bigMsg.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if _, _, err := bigMsg.ChooseDeliveryMethod(MethodPaper, DestinationTypeSingle); err == nil {
		t.Fatal("expected paper delivery to reject oversized payload")
	}
}

func TestDetermineTransportEncryption(t *testing.T) {
	cases := []struct {
		method    byte
		dstType   byte
		encrypted bool
		desc      string
	}{
		{MethodOpportunistic, DestinationTypeSingle, true, EncryptionDescriptionEC},
		{MethodOpportunistic, DestinationTypeGroup, true, EncryptionDescriptionAES},
		{MethodOpportunistic, DestinationTypePlain, false, EncryptionDescriptionUnencrypted},
		{MethodDirect, DestinationTypeSingle, true, EncryptionDescriptionEC},
		{MethodPropagated, DestinationTypeSingle, true, EncryptionDescriptionEC},
		{MethodPropagated, DestinationTypeGroup, true, EncryptionDescriptionAES},
		{MethodPaper, DestinationTypeSingle, true, EncryptionDescriptionEC},
		{MethodUnknown, DestinationTypeSingle, false, EncryptionDescriptionUnencrypted},
	}
	for _, c := range cases {
		enc, desc := DetermineTransportEncryption(c.method, c.dstType)
		if enc != c.encrypted {
			t.Errorf("method=%d dst=%d: encrypted want %v got %v", c.method, c.dstType, c.encrypted, enc)
		}
		if desc != c.desc {
			t.Errorf("method=%d dst=%d: description want %q got %q", c.method, c.dstType, c.desc, desc)
		}
	}
}

func TestValidateStamp_Ticket(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	msg, err := NewMessage(dst.Hash(), src.Hash(), nil, []byte("ticket"), nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := msg.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	ticket := bytes.Repeat([]byte{0x42}, TicketLength)
	msg.Stamp = truncatedHash(ticket, msg.Hash)

	ok, err := msg.ValidateStamp(8, [][]byte{ticket})
	if err != nil {
		t.Fatalf("ValidateStamp: %v", err)
	}
	if !ok {
		t.Fatal("ticket-derived stamp should validate")
	}
	if msg.StampValue != StampValueTicket {
		t.Errorf("expected StampValueTicket, got %d", msg.StampValue)
	}
}

func TestValidateStamp_NoStamp(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	msg, err := NewMessage(dst.Hash(), src.Hash(), nil, []byte("no-stamp"), nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := msg.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	ok, err := msg.ValidateStamp(8, nil)
	if err != nil {
		t.Fatalf("ValidateStamp: %v", err)
	}
	if ok {
		t.Fatal("missing stamp must not validate")
	}
}

func TestValidateStamp_RealProofOfWork(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	msg, err := NewMessage(dst.Hash(), src.Hash(), nil, []byte("pow"), nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := msg.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	const cost = 4
	wb, err := StampWorkblock(msg.Hash, WorkblockExpandRounds)
	if err != nil {
		t.Fatalf("workblock: %v", err)
	}
	stamp := make([]byte, StampSize)
	for n := uint32(0); ; n++ {
		stamp[0] = byte(n)
		stamp[1] = byte(n >> 8)
		stamp[2] = byte(n >> 16)
		stamp[3] = byte(n >> 24)
		if StampValid(stamp, cost, wb) {
			break
		}
		if n > 1_000_000 {
			t.Fatal("could not find low-cost stamp; flaky test")
		}
	}
	msg.Stamp = append([]byte(nil), stamp...)
	ok, err := msg.ValidateStamp(cost, nil)
	if err != nil {
		t.Fatalf("ValidateStamp: %v", err)
	}
	if !ok {
		t.Fatal("expected proof-of-work stamp to validate")
	}
	if msg.StampValue == 0 {
		t.Errorf("expected non-zero stamp value")
	}
}

func TestValidateStamp_RequiresHash(t *testing.T) {
	if _, err := (&LXMessage{}).ValidateStamp(8, nil); err == nil {
		t.Fatal("expected error when message has no hash")
	}
}

func TestLXMessage_DXAndNilSafety(t *testing.T) {
	// Nil receiver tests
	var nilMsg *LXMessage
	nilMsg.SetTitle("test")
	nilMsg.SetContent("test")
	if nilMsg.TitleString() != "" {
		t.Error("expected empty title for nil message")
	}
	if nilMsg.ContentString() != "" {
		t.Error("expected empty content for nil message")
	}
	if nilMsg.FormatHash() != "<nil>" {
		t.Errorf("expected <nil> hash representation, got %s", nilMsg.FormatHash())
	}
	if nilMsg.MessageID() != nil {
		t.Error("expected nil message ID")
	}
	if nilMsg.PackedSize() != 0 {
		t.Error("expected 0 packed size")
	}
	if _, err := nilMsg.ContentSize(); err == nil {
		t.Error("expected error for nil message content size")
	}
	if _, _, err := nilMsg.ChooseDeliveryMethod(0, 0); err == nil {
		t.Error("expected error for nil message choose delivery method")
	}
	if _, err := nilMsg.ValidateStamp(0, nil); err == nil {
		t.Error("expected error for nil message validate stamp")
	}
	if _, err := nilMsg.Pack(nil); err == nil {
		t.Error("expected error for nil message pack")
	}
	if _, err := nilMsg.EncryptedPayload(); err == nil {
		t.Error("expected error for nil message encrypted payload")
	}
	if err := nilMsg.PackPropagated(nil, 0); err == nil {
		t.Error("expected error for nil message pack propagated")
	}

	// Field getters/setters on nil receiver
	if _, ok := nilMsg.GetField(0x01); ok {
		t.Error("expected false for nil message GetField")
	}
	if nilMsg.GetFieldString(0x01) != "" {
		t.Error("expected empty string for nil message GetFieldString")
	}
	if nilMsg.GetFieldBytes(0x01) != nil {
		t.Error("expected nil bytes for nil message GetFieldBytes")
	}
	if nilMsg.GetFieldInt(0x01) != 0 {
		t.Error("expected 0 for nil message GetFieldInt")
	}
	if nilMsg.GetFieldBool(0x01) != false {
		t.Error("expected false for nil message GetFieldBool")
	}
	nilMsg.SetField(0x01, "val") // should not panic

	// Valid message field getters/setters
	msg := &LXMessage{}
	msg.SetField(0x01, "hello")
	if msg.Fields == nil {
		t.Error("expected Fields map to be initialized after SetField")
	}
	msg.SetField(0x02, []byte("world"))
	msg.SetField(0x03, int64(42))
	msg.SetField(0x04, true)

	if v, ok := msg.GetField(0x01); !ok || v != "hello" {
		t.Errorf("GetField failed: got %v, ok %v", v, ok)
	}
	if msg.GetFieldString(0x01) != "hello" {
		t.Errorf("GetFieldString failed: got %q", msg.GetFieldString(0x01))
	}
	if msg.GetFieldString(0x02) != "world" {
		t.Errorf("GetFieldString with bytes failed: got %q", msg.GetFieldString(0x02))
	}
	if !bytes.Equal(msg.GetFieldBytes(0x02), []byte("world")) {
		t.Errorf("GetFieldBytes failed: got %v", msg.GetFieldBytes(0x02))
	}
	if !bytes.Equal(msg.GetFieldBytes(0x01), []byte("hello")) {
		t.Errorf("GetFieldBytes with string failed: got %v", msg.GetFieldBytes(0x01))
	}
	if msg.GetFieldInt(0x03) != 42 {
		t.Errorf("GetFieldInt failed: got %d", msg.GetFieldInt(0x03))
	}
	if msg.GetFieldBool(0x04) != true {
		t.Errorf("GetFieldBool failed: got %v", msg.GetFieldBool(0x04))
	}

	// Uninitialized message Fields map auto-initialization
	msg2 := &LXMessage{Fields: nil}
	msg2.SetField(0x01, "auto-init")
	if msg2.Fields == nil {
		t.Error("expected Fields map to be auto-initialized")
	}
	if msg2.GetFieldString(0x01) != "auto-init" {
		t.Errorf("GetFieldString failed after auto-init: got %q", msg2.GetFieldString(0x01))
	}
}

func TestLXMessage_Diagnostics(t *testing.T) {
	var nilMsg *LXMessage
	if got := nilMsg.Diagnostics(); got != "LXMessage: <nil>" {
		t.Errorf("expected nil diagnostics, got %q", got)
	}

	msg := &LXMessage{
		DestinationHash: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SourceHash:      []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		Title:           []byte("Diag Subject"),
		Content:         []byte("Diag Body"),
		Timestamp:       1781234567.123456,
		State:           StateSent,
		Method:          MethodDirect,
		Representation:  RepresentationPacket,
	}
	msg.SetField(FieldRenderer, "markdown")
	msg.SetField(FieldDebug, "true")

	diag := msg.Diagnostics()
	if !strings.Contains(diag, "LXMF Message Diagnostics") {
		t.Error("expected diagnostics header")
	}
	if !strings.Contains(diag, "Diag Subject") {
		t.Error("expected title in diagnostics")
	}
	if !strings.Contains(diag, "Renderer") {
		t.Error("expected field name Renderer in diagnostics")
	}
	if !strings.Contains(diag, "Debug") {
		t.Error("expected field name Debug in diagnostics")
	}
	if !strings.Contains(diag, "State:              Sent") {
		t.Error("expected State: Sent in diagnostics")
	}
	if !strings.Contains(diag, "Delivery Method:    Direct") {
		t.Error("expected Delivery Method: Direct in diagnostics")
	}
	if !strings.Contains(diag, "Representation:     Packet") {
		t.Error("expected Representation: Packet in diagnostics")
	}

	// Test helper string functions
	if StateString(StateGenerating) != "Generating" {
		t.Error("StateString failed")
	}
	if StateString(0x99) != "Unknown(0x99)" {
		t.Error("StateString unknown failed")
	}
	if MethodString(MethodPropagated) != "Propagated" {
		t.Error("MethodString failed")
	}
	if MethodString(0x99) != "Unknown(0x99)" {
		t.Error("MethodString unknown failed")
	}
	if RepresentationString(RepresentationResource) != "Resource" {
		t.Error("RepresentationString failed")
	}
	if RepresentationString(0x99) != "Unknown(0x99)" {
		t.Error("RepresentationString unknown failed")
	}
	if FieldName(FieldAudio) != "Audio" {
		t.Error("FieldName failed")
	}
	if FieldName(0x99) != "Custom(0x99)" {
		t.Error("FieldName unknown failed")
	}
}

var _ = identity.NewIdentity
