// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"testing"
	"testing/quick"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

func TestPackUnpack_RoundTripRandomized(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)

	f := func(titleLen, contentLen uint8, tsBits uint32) bool {
		title := bytes.Repeat([]byte{'T'}, int(titleLen%64))
		content := bytes.Repeat([]byte{'C'}, int(contentLen%128))
		msg, err := NewMessage(dst.Hash(), src.Hash(), title, content, nil)
		if err != nil {
			return false
		}
		msg.Timestamp = float64(tsBits%1_000_000) + 1_700_000_000
		packed, err := msg.Pack(src)
		if err != nil {
			return false
		}
		got, err := Unpack(packed, RecallSource)
		if err != nil {
			return false
		}
		return bytes.Equal(got.Title, title) && bytes.Equal(got.Content, content) &&
			bytes.Equal(got.DestinationHash, dst.Hash()) && bytes.Equal(got.SourceHash, src.Hash())
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 80}); err != nil {
		t.Fatal(err)
	}
}

func TestPaperURI_RoundTripRandomized(t *testing.T) {
	f := func(n uint16) bool {
		size := int(n % uint16(PaperMDU))
		if size == 0 {
			size = 1
		}
		payload := bytes.Repeat([]byte{0x5a}, size)
		uri, err := PaperURI(payload)
		if err != nil {
			return false
		}
		got, err := DecodePaperURI(uri)
		if err != nil {
			return false
		}
		return bytes.Equal(got, payload)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 40}); err != nil {
		t.Fatal(err)
	}
}

func TestHexHash_LengthMatchesInput(t *testing.T) {
	f := func(b []byte) bool {
		if len(b) > 64 {
			b = b[:64]
		}
		h := HexHash(b)
		if len(b) == 0 {
			return h == "<empty>"
		}
		return len(h) == len(b)*2
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

func TestMessageStoreFilename_RoundTripRandomized(t *testing.T) {
	f := func(n byte, received uint16, stamp int8) bool {
		id := bytes.Repeat([]byte{n}, 8+int(n%8))
		name := MessageStoreFilename(id, float64(received), int64(stamp))
		gotID, ts, st, err := ParseMessageStoreFilename(name)
		if err != nil {
			return false
		}
		return bytes.Equal(gotID, id) && ts == float64(received) && st == int64(stamp)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestPackUnpack_RepackStable(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)

	cases := []struct {
		title, content string
		fields         map[byte]any
	}{
		{"", "plain", nil},
		{"subj", "body", nil},
		{"r", "x", map[byte]any{FieldRenderer: "markdown"}},
	}
	for _, tc := range cases {
		msg, err := NewMessage(dst.Hash(), src.Hash(), []byte(tc.title), []byte(tc.content), tc.fields)
		if err != nil {
			t.Fatal(err)
		}
		msg.Timestamp = 1_800_000_000
		raw1 := mustPack(t, msg, src)
		mid, err := Unpack(raw1, RecallSource)
		if err != nil {
			t.Fatal(err)
		}
		if mid.TitleString() != tc.title || mid.ContentString() != tc.content {
			t.Fatalf("first decode drift: %+v", tc)
		}
		repacked := mustPack(t, mid, src)
		end, err := Unpack(repacked, RecallSource)
		if err != nil {
			t.Fatal(err)
		}
		if end.TitleString() != tc.title || end.ContentString() != tc.content {
			t.Fatalf("repack drift: %+v", tc)
		}
	}
}

func TestPack_DestinationSourcePrefixes(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	msg, err := NewMessage(dst.Hash(), src.Hash(), []byte("a"), []byte("b"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw := mustPack(t, msg, src)
	if !bytes.Equal(raw[:DestinationLength], dst.Hash()) {
		t.Fatal("destination hash prefix mismatch")
	}
	if !bytes.Equal(raw[DestinationLength:2*DestinationLength], src.Hash()) {
		t.Fatal("source hash prefix mismatch")
	}
	if len(raw) < Overhead {
		t.Fatalf("len=%d < overhead=%d", len(raw), Overhead)
	}
}

func TestStampCostFromAppData_ReadsV5Cost(t *testing.T) {
	raw, err := EncodeAnnounceAppDataV5("hub", 12)
	if err != nil {
		t.Fatal(err)
	}
	cost, ok, err := StampCostFromAppData(raw)
	if err != nil || !ok || cost != 12 {
		t.Fatalf("cost=%d ok=%v err=%v", cost, ok, err)
	}
	t.Log("LXMF_ANNOUNCE_COST_PROVED")
}

func TestFieldConstants_MatchUpstream(t *testing.T) {
	if FieldEmbeddedLXMs != 0x01 || FieldRenderer != 0x0F || FieldReplyTo != 0x30 || FieldReaction != 0x40 {
		t.Fatal("core field ids")
	}
	if FieldCustomType != 0xFB || FieldDebug != 0xFF {
		t.Fatal("custom field ids")
	}
	if AudioCodec2_450PWB != 0x01 || AudioOpusOgg != 0x10 || RendererMarkdown != 0x02 {
		t.Fatal("mode constants")
	}
	if FieldTelemetry != 0x02 || FieldFileAttachments != 0x05 || FieldAudio != 0x07 || FieldTicket != 0x0C {
		t.Fatal("mid field ids")
	}
	if FieldComment != 0x41 || FieldContinuation != 0x42 || FieldCustomData != 0xFC {
		t.Fatal("comment/custom field ids")
	}
	if ReactionTo != 0x00 || CommentFor != 0x00 || ContinuationOf != 0x00 {
		t.Fatal("nested dict keys")
	}
	if PNMetaVersion != 0x00 || PNMetaUtilPressure != 0x05 || SFCompression != 0x00 {
		t.Fatal("pn/sf keys")
	}
	if MethodPaper != 0x05 || URISchema != "lxm" || AppName != "lxmf" || Version != "1.3.0" {
		t.Fatal("method/uri/version")
	}
	t.Log("LXMF_FIELD_CONSTANTS_PROVED")
}
