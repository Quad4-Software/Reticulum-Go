// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"maps"
	"testing"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

func TestDisplayNameFromAppData_LegacyPlain(t *testing.T) {
	got, err := DisplayNameFromAppData([]byte("alice"))
	if err != nil {
		t.Fatalf("DisplayNameFromAppData: %v", err)
	}
	if got != "alice" {
		t.Errorf("got %q, want alice", got)
	}
}

func TestDisplayNameFromAppData_Empty(t *testing.T) {
	got, err := DisplayNameFromAppData(nil)
	if err != nil || got != "" {
		t.Fatalf("expected empty, got %q err=%v", got, err)
	}
}

func TestDisplayNameFromAppData_V5(t *testing.T) {
	encoded, err := EncodeAnnounceAppDataV5("bob", 32)
	if err != nil {
		t.Fatalf("encode v5: %v", err)
	}
	got, err := DisplayNameFromAppData(encoded)
	if err != nil {
		t.Fatalf("DisplayNameFromAppData: %v", err)
	}
	if got != "bob" {
		t.Errorf("got %q, want bob", got)
	}
}

func TestDisplayNameFromAppData_V5SanitizesNullsAndSpace(t *testing.T) {
	payload := []any{[]byte("  peer\x00name  "), int64(8)}
	blob, err := marshalAnnounceAppData(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := DisplayNameFromAppData(blob)
	if err != nil {
		t.Fatalf("DisplayNameFromAppData: %v", err)
	}
	if got != "peername" {
		t.Errorf("got %q", got)
	}
}

func TestStampCostFromAppData(t *testing.T) {
	encoded, err := EncodeAnnounceAppDataV5("c", 64)
	if err != nil {
		t.Fatalf("encode v5: %v", err)
	}
	cost, ok, err := StampCostFromAppData(encoded)
	if err != nil {
		t.Fatalf("StampCostFromAppData: %v", err)
	}
	if !ok || cost != 64 {
		t.Errorf("got cost=%d ok=%v", cost, ok)
	}

	cost, ok, err = StampCostFromAppData([]byte("legacy"))
	if err != nil {
		t.Fatalf("legacy err: %v", err)
	}
	if ok {
		t.Errorf("legacy must not advertise stamp cost; got %d", cost)
	}
}

func TestEncodeAnnounceAppData_Legacy(t *testing.T) {
	if got := EncodeAnnounceAppData("hello"); string(got) != "hello" {
		t.Errorf("legacy encode mismatch: %q", got)
	}
}

func TestEncodeAnnounceAppDataWithIcon(t *testing.T) {
	raw, err := EncodeAnnounceAppDataWithIcon("alice", 8, "hiking", []byte{0xff, 0xff, 0x00}, []byte{0x00, 0x00, 0xff})
	if err != nil {
		t.Fatal(err)
	}
	cost, ok, err := StampCostFromAppData(raw)
	if err != nil || !ok || cost != 8 {
		t.Fatalf("cost=%d ok=%v err=%v", cost, ok, err)
	}
	arr, err := unmarshalMsgpackArray(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(arr) != 3 {
		t.Fatalf("len=%d want 3", len(arr))
	}
}

func TestEncodeAnnounceAppDataV5_OmitsCost(t *testing.T) {
	encoded, err := EncodeAnnounceAppDataV5("dave", -1)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	cost, ok, err := StampCostFromAppData(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ok {
		t.Errorf("expected no stamp cost, got %d", cost)
	}
}

func TestCompressionSupportFromAppData_EmptyOrNil(t *testing.T) {
	for _, in := range [][]byte{nil, {}} {
		support, ok, err := CompressionSupportFromAppData(in)
		if err != nil {
			t.Fatalf("CompressionSupportFromAppData: %v", err)
		}
		if ok {
			t.Errorf("expected ok=false for empty input, got support=%v", support)
		}
	}
}

func TestCompressionSupportFromAppData_LegacyImpliesTrue(t *testing.T) {
	support, ok, err := CompressionSupportFromAppData(EncodeAnnounceAppData("legacy"))
	if err != nil {
		t.Fatalf("CompressionSupportFromAppData: %v", err)
	}
	if !ok || !support {
		t.Errorf("legacy app data must default to supported=true, got support=%v ok=%v", support, ok)
	}
}

func TestCompressionSupportFromAppData_V5DefaultsTrue(t *testing.T) {
	encoded, err := EncodeAnnounceAppDataV5("v5-default", 8)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	support, ok, err := CompressionSupportFromAppData(encoded)
	if err != nil {
		t.Fatalf("CompressionSupportFromAppData: %v", err)
	}
	if !ok || !support {
		t.Errorf("v5 app data without functionality element must report supported=true, got support=%v ok=%v", support, ok)
	}
}

func TestCompressionSupportFromAppData_FeaturePresent(t *testing.T) {
	encoded, err := EncodeAnnounceAppDataV5WithFeatures("v5-flag", 8, []byte{SFCompression})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	support, ok, err := CompressionSupportFromAppData(encoded)
	if err != nil {
		t.Fatalf("CompressionSupportFromAppData: %v", err)
	}
	if !ok || !support {
		t.Errorf("explicit SFCompression must report supported=true, got support=%v ok=%v", support, ok)
	}
}

func TestCompressionSupportFromAppData_FeatureAbsent(t *testing.T) {
	encoded, err := EncodeAnnounceAppDataV5WithFeatures("microR", 8, []byte{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	support, ok, err := CompressionSupportFromAppData(encoded)
	if err != nil {
		t.Fatalf("CompressionSupportFromAppData: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for v5 announce with explicit empty feature list")
	}
	if support {
		t.Error("empty feature list must report supported=false (peer omitted SFCompression)")
	}
}

func TestFeatureSupportFromAppData(t *testing.T) {
	encoded, err := EncodeAnnounceAppDataV5WithFeatures("peer", 0, []byte{SFCompression})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	codes, ok, err := FeatureSupportFromAppData(encoded)
	if err != nil {
		t.Fatalf("FeatureSupportFromAppData: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true with explicit feature element")
	}
	if len(codes) != 1 || codes[0] != SFCompression {
		t.Errorf("unexpected feature codes: %v", codes)
	}

	encoded2, err := EncodeAnnounceAppDataV5("no-features", 0)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	codes, ok, err = FeatureSupportFromAppData(encoded2)
	if err != nil {
		t.Fatalf("FeatureSupportFromAppData: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false when feature element omitted, got %v", codes)
	}
}

func TestEncodeAnnounceAppDataV5WithFeatures_StampCostStillReadable(t *testing.T) {
	encoded, err := EncodeAnnounceAppDataV5WithFeatures("peer", 64, []byte{SFCompression})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	cost, ok, err := StampCostFromAppData(encoded)
	if err != nil {
		t.Fatalf("StampCostFromAppData: %v", err)
	}
	if !ok || cost != 64 {
		t.Errorf("stamp cost mismatch with features element: got %d ok=%v", cost, ok)
	}
	got, err := DisplayNameFromAppData(encoded)
	if err != nil {
		t.Fatalf("DisplayNameFromAppData: %v", err)
	}
	if got != "peer" {
		t.Errorf("display name mismatch: %q", got)
	}
}

func buildPNAnnounce(t *testing.T, timebase int64, isPN bool, transferLimit, syncLimit, targetCost, costFlex, peeringCost int64, metadata map[byte]any) []byte {
	t.Helper()
	md := map[byte]any{}
	maps.Copy(md, metadata)
	payload := []any{
		nil,
		timebase,
		isPN,
		transferLimit,
		syncLimit,
		[]any{targetCost, costFlex, peeringCost},
		md,
	}
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	enc.UseCompactInts(true)
	if err := enc.Encode(payload); err != nil {
		t.Fatalf("encode pn announce: %v", err)
	}
	return buf.Bytes()
}

func TestPNAnnounceDataIsValid(t *testing.T) {
	good := buildPNAnnounce(t, 1700000000, true, 256, 10240, 16, 3, 18, map[byte]any{
		PNMetaName: []byte("test-node"),
	})
	if !PNAnnounceDataIsValid(good) {
		t.Fatal("expected valid PN announce")
	}
	if PNAnnounceDataIsValid(nil) {
		t.Error("expected nil to be invalid")
	}
	if PNAnnounceDataIsValid([]byte("not msgpack")) {
		t.Error("expected garbage to be invalid")
	}
}

func TestPNStampCostFromAppData(t *testing.T) {
	blob := buildPNAnnounce(t, 1, true, 256, 10240, 17, 3, 18, nil)
	cost, ok, err := PNStampCostFromAppData(blob)
	if err != nil {
		t.Fatalf("PNStampCostFromAppData: %v", err)
	}
	if !ok || cost != 17 {
		t.Errorf("got cost=%d ok=%v", cost, ok)
	}
}

func TestPNNameFromAppData(t *testing.T) {
	blob := buildPNAnnounce(t, 1, true, 256, 10240, 16, 3, 18, map[byte]any{
		PNMetaName: []byte("propnode"),
	})
	name, ok, err := PNNameFromAppData(blob)
	if err != nil {
		t.Fatalf("PNNameFromAppData: %v", err)
	}
	if !ok || name != "propnode" {
		t.Errorf("got name=%q ok=%v", name, ok)
	}
}

func TestPNNameFromAppData_Missing(t *testing.T) {
	blob := buildPNAnnounce(t, 1, true, 256, 10240, 16, 3, 18, nil)
	name, ok, err := PNNameFromAppData(blob)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok || name != "" {
		t.Errorf("expected absent name; got %q ok=%v", name, ok)
	}
}
