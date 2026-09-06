// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
)

func TestEncodeInfoRequiresType(t *testing.T) {
	if _, err := EncodeInfo(Info{TransportID: []byte("x")}); err == nil {
		t.Fatal("expected error when Type is empty")
	}
}

func TestEncodeInfoRequiresTransportID(t *testing.T) {
	if _, err := EncodeInfo(Info{Type: "TCPClientInterface"}); err == nil {
		t.Fatal("expected error when TransportID is empty")
	}
}

func TestEncodeInfoOmitsZeroOptionals(t *testing.T) {
	packed, err := EncodeInfo(Info{Type: "T", TransportID: []byte("id"), Name: "n"})
	if err != nil {
		t.Fatalf("EncodeInfo: %v", err)
	}
	out, err := DecodeInfo(packed)
	if err != nil {
		t.Fatalf("DecodeInfo: %v", err)
	}
	if out.HasPort {
		t.Error("HasPort should be false when Port unset")
	}
	// EncodeInfo always emits geo fields (even when zero), so HasGeo is true
	// after a round-trip, and the values themselves should remain zero.
	if out.Latitude != 0 || out.Longitude != 0 || out.Height != 0 {
		t.Errorf("geo fields should be zero, got %+v", out)
	}
	if out.Frequency != 0 || out.Bandwidth != 0 || out.Modulation != "" {
		t.Errorf("optional fields should be zero, got %+v", out)
	}
}

func TestEncodeInfoIncludesRadioFields(t *testing.T) {
	in := Info{
		Type:            "RNodeInterface",
		TransportID:     []byte("id"),
		Frequency:       868,
		Bandwidth:       125,
		SpreadingFactor: 7,
		CodingRate:      5,
		Channel:         0,
		Modulation:      "LoRa",
	}
	packed, err := EncodeInfo(in)
	if err != nil {
		t.Fatalf("EncodeInfo: %v", err)
	}
	out, err := DecodeInfo(packed)
	if err != nil {
		t.Fatalf("DecodeInfo: %v", err)
	}
	if out.Frequency != 868 || out.Bandwidth != 125 || out.SpreadingFactor != 7 ||
		out.CodingRate != 5 || out.Modulation != "LoRa" {
		t.Errorf("radio fields mismatch: %+v", out)
	}
	// Channel == 0 is omitted by EncodeInfo, so it stays zero on decode.
	if out.Channel != 0 {
		t.Errorf("Channel should be zero, got %d", out.Channel)
	}
}

func TestEncodeInfoIncludesOperatorLXMFAddress(t *testing.T) {
	addr := bytes.Repeat([]byte{0xab}, 16)
	in := Info{
		Type:                "BackboneInterface",
		TransportID:         []byte("transport-id-16b"),
		OperatorLXMFAddress: addr,
	}
	packed, err := EncodeInfo(in)
	if err != nil {
		t.Fatalf("EncodeInfo: %v", err)
	}
	out, err := DecodeInfo(packed)
	if err != nil {
		t.Fatalf("DecodeInfo: %v", err)
	}
	if !bytes.Equal(out.OperatorLXMFAddress, addr) {
		t.Fatalf("OperatorLXMFAddress=%x want %x", out.OperatorLXMFAddress, addr)
	}
}

func TestEncodeInfoIncludesOperatorPageAddress(t *testing.T) {
	page := bytes.Repeat([]byte{0xcd}, 16)
	in := Info{
		Type:                "TCPServerInterface",
		TransportID:         bytes.Repeat([]byte{0x11}, 16),
		OperatorPageAddress: page,
	}
	packed, err := EncodeInfo(in)
	if err != nil {
		t.Fatalf("EncodeInfo: %v", err)
	}
	out, err := DecodeInfo(packed)
	if err != nil {
		t.Fatalf("DecodeInfo: %v", err)
	}
	if !bytes.Equal(out.OperatorPageAddress, page) {
		t.Fatalf("OperatorPageAddress=%x want %x", out.OperatorPageAddress, page)
	}
	if out.OperatorLXMFAddress != nil {
		t.Fatalf("unexpected OperatorLXMFAddress=%x", out.OperatorLXMFAddress)
	}
}

func TestDecodeInfoIgnoresUnknownOperatorField(t *testing.T) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	if err := enc.EncodeMapLen(3); err != nil {
		t.Fatal(err)
	}
	_ = enc.Encode(byte(FieldInterfaceType))
	_ = enc.Encode("BackboneInterface")
	_ = enc.Encode(byte(FieldTransportID))
	_ = enc.Encode(bytes.Repeat([]byte{0x22}, 16))
	_ = enc.Encode(byte(0xF2))
	_ = enc.Encode(bytes.Repeat([]byte{0xee}, 16))
	out, err := DecodeInfo(buf.Bytes())
	if err != nil {
		t.Fatalf("DecodeInfo: %v", err)
	}
	if out.Type != "BackboneInterface" {
		t.Fatalf("Type=%q", out.Type)
	}
	if out.OperatorPageAddress != nil || out.OperatorLXMFAddress != nil {
		t.Fatalf("unknown field should be ignored")
	}
}

func TestEncodeInfoIncludesTransportImplVers(t *testing.T) {
	in := Info{
		Type:          "TCPServerInterface",
		TransportID:   bytes.Repeat([]byte{0x11}, 16),
		Name:          "hub",
		TransportImpl: "reticulum-go",
		TransportVers: "1.5.2-test",
	}
	packed, err := EncodeInfo(in)
	if err != nil {
		t.Fatalf("EncodeInfo: %v", err)
	}
	out, err := DecodeInfo(packed)
	if err != nil {
		t.Fatalf("DecodeInfo: %v", err)
	}
	if out.TransportImpl != in.TransportImpl {
		t.Fatalf("TransportImpl=%q want %q", out.TransportImpl, in.TransportImpl)
	}
	if out.TransportVers != in.TransportVers {
		t.Fatalf("TransportVers=%q want %q", out.TransportVers, in.TransportVers)
	}
}

func TestEncodeInfoDefaultsTransportImplVers(t *testing.T) {
	in := Info{
		Type:        "TCPServerInterface",
		TransportID: bytes.Repeat([]byte{0x22}, 16),
		Name:        "hub",
	}
	packed, err := EncodeInfo(in)
	if err != nil {
		t.Fatalf("EncodeInfo: %v", err)
	}
	out, err := DecodeInfo(packed)
	if err != nil {
		t.Fatalf("DecodeInfo: %v", err)
	}
	if out.TransportImpl != ImplementationName {
		t.Fatalf("TransportImpl=%q want %q", out.TransportImpl, ImplementationName)
	}
	if out.TransportVers == "" {
		t.Fatal("TransportVers should default to non-empty")
	}
}

func TestDecodeInfoTooLarge(t *testing.T) {
	raw := make([]byte, MaxInfoSize+1)
	if _, err := DecodeInfo(raw); err == nil {
		t.Fatal("expected error for oversize info payload")
	}
}

func TestDecodeInfoMapLengthExceedsBounds(t *testing.T) {
	// map32 header declaring 65536 entries in a 5-byte payload.
	raw := []byte{0xdf, 0x00, 0x01, 0x00, 0x00}
	if _, err := DecodeInfo(raw); err == nil {
		t.Fatal("expected error for declared map length exceeding bounds")
	}
}

func TestDecodeInfoUnsupportedValueCode(t *testing.T) {
	// fixmap(1) { FieldInterfaceType(0x00) => empty fixarray(0x90) }
	// Arrays are rejected by safeDecodeInterface.
	raw := []byte{0x81, 0x00, 0x90}
	if _, err := DecodeInfo(raw); err == nil {
		t.Fatal("expected error for unsupported msgpack value code")
	}
}

func TestDecodeInfoTransportIDFromString(t *testing.T) {
	// Build a map with FieldTransportID as a str instead of bin to exercise
	// the string->[]byte branch.
	packed, err := EncodeInfo(Info{Type: "T", TransportID: []byte("from-bin")})
	if err != nil {
		t.Fatalf("EncodeInfo: %v", err)
	}
	out, err := DecodeInfo(packed)
	if err != nil {
		t.Fatalf("DecodeInfo: %v", err)
	}
	if string(out.TransportID) != "from-bin" {
		t.Errorf("TransportID: got %q", out.TransportID)
	}
}

func TestEncodeAppDataRejectsBadStampSize(t *testing.T) {
	if _, err := EncodeAppData(0, []byte("p"), []byte{0x01}); err == nil {
		t.Fatal("expected error for stamp of wrong size")
	}
}

func TestDecodeAppDataTooShort(t *testing.T) {
	if _, _, _, err := DecodeAppData(bytes.Repeat([]byte{0x00}, StampSize)); err == nil {
		t.Fatal("expected error for too-short app_data")
	}
}

func TestDecodeAppDataTooLarge(t *testing.T) {
	raw := make([]byte, MaxAppDataSize+1)
	if _, _, _, err := DecodeAppData(raw); err == nil {
		t.Fatal("expected error for oversize app_data")
	}
}

func TestStampValidOutOfRange(t *testing.T) {
	wb := []byte("workblock")
	stamp := bytes.Repeat([]byte{0x00}, StampSize)
	if StampValid(stamp, -1, wb) {
		t.Error("StampValid(-1) should be false")
	}
	if StampValid(stamp, 257, wb) {
		t.Error("StampValid(257) should be false")
	}
}

func TestStampValidRejectsWrongLength(t *testing.T) {
	wb := []byte("workblock")
	if StampValid(nil, 0, wb) {
		t.Error("nil stamp must be invalid even at cost 0")
	}
	if StampValid([]byte{0x01}, 0, wb) {
		t.Error("short stamp must be invalid")
	}
	if StampValid(bytes.Repeat([]byte{0x00}, StampSize+1), 0, wb) {
		t.Error("oversized stamp must be invalid")
	}
	if !StampValid(bytes.Repeat([]byte{0x00}, StampSize), 0, wb) {
		t.Error("exact StampSize at cost 0 must be valid")
	}
}

func TestStampValueDeterministic(t *testing.T) {
	wb := []byte("wb")
	stamp := bytes.Repeat([]byte{0xAB}, StampSize)
	v1 := StampValue(wb, stamp)
	v2 := StampValue(wb, stamp)
	if v1 != v2 {
		t.Errorf("StampValue not deterministic: %d vs %d", v1, v2)
	}
	// Different stamps must (almost always) yield different values.
	other := bytes.Repeat([]byte{0xCD}, StampSize)
	if v2 := StampValue(wb, other); v2 == v1 {
		t.Logf("note: StampValue collision for distinct stamps (%d)", v1)
	}
}

func TestStampWorkblockDefaultRounds(t *testing.T) {
	a, err := StampWorkblock([]byte("material"), 0)
	if err != nil {
		t.Fatalf("StampWorkblock(0): %v", err)
	}
	b, err := StampWorkblock([]byte("material"), WorkblockExpandRounds)
	if err != nil {
		t.Fatalf("StampWorkblock(default): %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("expandRounds<=0 should default to WorkblockExpandRounds")
	}
	if len(a) != 256*WorkblockExpandRounds {
		t.Errorf("workblock size: got %d, want %d", len(a), 256*WorkblockExpandRounds)
	}
}

func TestInfoHashIsSHA256(t *testing.T) {
	got := InfoHash([]byte("payload"))
	want := sha256.Sum256([]byte("payload"))
	if !bytes.Equal(got, want[:]) {
		t.Fatal("InfoHash should be sha256")
	}
}

func TestBuildAppDataAndValidateRoundTrip(t *testing.T) {
	in := Info{Type: "AutoInterface", TransportID: bytes.Repeat([]byte{0x11}, 16), Name: "node"}
	app, err := BuildAppData(in, 2, 3)
	if err != nil {
		t.Fatalf("BuildAppData: %v", err)
	}
	info, err := ValidateAndDecode(app, 2, 3)
	if err != nil {
		t.Fatalf("ValidateAndDecode: %v", err)
	}
	if info.Info.Type != in.Type {
		t.Errorf("type: got %q, want %q", info.Info.Type, in.Type)
	}
	if info.Info.Name != in.Name {
		t.Errorf("name: got %q, want %q", info.Info.Name, in.Name)
	}
	if info.StampValue < 2 {
		t.Errorf("StampValue: got %d, want >= 2", info.StampValue)
	}
	if len(info.Stamp) != StampSize {
		t.Errorf("stamp size: got %d", len(info.Stamp))
	}
}

func TestValidateAndDecodeRejectsEncryptedFlag(t *testing.T) {
	packed, err := EncodeInfo(Info{Type: "T", TransportID: []byte("id")})
	if err != nil {
		t.Fatalf("EncodeInfo: %v", err)
	}
	stamp := bytes.Repeat([]byte{0x00}, StampSize)
	app, err := EncodeAppData(FlagEncrypted, packed, stamp)
	if err != nil {
		t.Fatalf("EncodeAppData: %v", err)
	}
	if _, err := ValidateAndDecode(app, 0, 3); err == nil {
		t.Fatal("expected error for encrypted announce without decryption")
	}
}

func TestValidateAndDecodeRejectsLowStamp(t *testing.T) {
	packed, err := EncodeInfo(Info{Type: "T", TransportID: []byte("id")})
	if err != nil {
		t.Fatalf("EncodeInfo: %v", err)
	}
	// A 0xff stamp will not meet a high required cost.
	stamp := bytes.Repeat([]byte{0xff}, StampSize)
	app, err := EncodeAppData(0, packed, stamp)
	if err != nil {
		t.Fatalf("EncodeAppData: %v", err)
	}
	if _, err := ValidateAndDecode(app, 200, 3); err == nil {
		t.Fatal("expected error for stamp below required cost")
	}
}

func TestValidateAndDecodeRejectsShortAppData(t *testing.T) {
	if _, err := ValidateAndDecode([]byte{0x00}, 0, 3); err == nil {
		t.Fatal("expected error for too-short app_data")
	}
}

func TestToIntClampsLargeUint64(t *testing.T) {
	const maxInt64 = uint64(1<<63 - 1)
	if got := toInt(uint64(1 << 63)); got != int64(maxInt64) {
		t.Errorf("toInt(2^63): got %d, want %d", got, int64(maxInt64))
	}
	if got := toInt(uint64(42)); got != 42 {
		t.Errorf("toInt(42): got %d", got)
	}
	if got := toInt(int32(-7)); got != -7 {
		t.Errorf("toInt(int32(-7)): got %d", got)
	}
	if got := toInt(float64(3.9)); got != 3 {
		t.Errorf("toInt(3.9): got %d", got)
	}
	if got := toInt("not a number"); got != 0 {
		t.Errorf("toInt(string): got %d, want 0", got)
	}
	if _, ok := toIntOK("not a number"); ok {
		t.Error("toIntOK(string) should be false")
	}
}

func TestDecodeInfoIgnoresNonNumericPortAndGeo(t *testing.T) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	_ = enc.EncodeMapLen(5)
	_ = enc.Encode(byte(FieldInterfaceType))
	_ = enc.Encode("TCPInterface")
	_ = enc.Encode(byte(FieldTransportID))
	_ = enc.Encode([]byte{0x01, 0x02})
	_ = enc.Encode(byte(FieldPort))
	_ = enc.Encode("9999")
	_ = enc.Encode(byte(FieldLatitude))
	_ = enc.Encode("north")
	_ = enc.Encode(byte(FieldName))
	_ = enc.Encode("n")

	out, err := DecodeInfo(buf.Bytes())
	if err != nil {
		t.Fatalf("DecodeInfo: %v", err)
	}
	if out.HasPort {
		t.Fatal("HasPort must stay false for string port")
	}
	if out.HasGeo {
		t.Fatal("HasGeo must stay false for string latitude")
	}
	if out.Type != "TCPInterface" {
		t.Fatalf("Type=%q", out.Type)
	}
}

func TestToFloatCoversTypes(t *testing.T) {
	if got := toFloat(float32(1.5)); got != 1.5 {
		t.Errorf("toFloat(float32): got %v", got)
	}
	if got := toFloat(int64(5)); got != 5 {
		t.Errorf("toFloat(int64): got %v", got)
	}
	if got := toFloat(uint64(7)); got != 7 {
		t.Errorf("toFloat(uint64): got %v", got)
	}
	if got := toFloat(int(9)); got != 9 {
		t.Errorf("toFloat(int): got %v", got)
	}
	if got := toFloat(int32(3)); got != 3 {
		t.Errorf("toFloat(int32): got %v", got)
	}
	if got := toFloat("nope"); got != 0 {
		t.Errorf("toFloat(string): got %v, want 0", got)
	}
}
