// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"errors"
	"testing"
)

func TestUnit_AppIdentityConstants(t *testing.T) {
	if AppName != "lxmf" {
		t.Fatalf("AppName=%q", AppName)
	}
	if DestinationLength != 16 || SignatureLength != 64 {
		t.Fatal("wire sizes")
	}
	if URISchema != "lxm" {
		t.Fatalf("URISchema=%q", URISchema)
	}
}

func TestUnit_UnpackNilAndEmpty(t *testing.T) {
	if _, err := Unpack(nil, RecallSource); !errors.Is(err, ErrMessageTooShort) {
		t.Fatalf("nil: %v", err)
	}
	if _, err := Unpack([]byte{}, RecallSource); !errors.Is(err, ErrMessageTooShort) {
		t.Fatalf("empty: %v", err)
	}
}

func TestUnit_UnpackFromBytesBadDestination(t *testing.T) {
	_, err := UnpackFromBytes([]byte{1, 2}, bytes.Repeat([]byte{0}, DestinationLength+SignatureLength), RecallSource)
	if !errors.Is(err, ErrInvalidHashLength) {
		t.Fatalf("err=%v", err)
	}
}

func TestUnit_PackContainerNil(t *testing.T) {
	if _, err := PackContainer(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestUnit_HexHashEmpty(t *testing.T) {
	if HexHash(nil) != "<empty>" {
		t.Fatal(HexHash(nil))
	}
}

func TestUnit_PaperURIEmpty(t *testing.T) {
	if _, err := PaperURI(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestUnit_StampWorkblockRejectsBadRounds(t *testing.T) {
	_, err := StampWorkblock([]byte("x"), 0)
	if err == nil {
		t.Fatal("expected error for expandRounds=0")
	}
}

func TestUnit_StampWorkblockEmptyMaterial(t *testing.T) {
	_, err := StampWorkblock(nil, 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnit_MessageStoreFilenameRoundTrip(t *testing.T) {
	id := bytes.Repeat([]byte{0xab}, 16)
	name := MessageStoreFilename(id, 1.5, 7)
	gotID, ts, stamp, err := ParseMessageStoreFilename(name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotID, id) || ts != 1.5 || stamp != 7 {
		t.Fatalf("id=%x ts=%v stamp=%d", gotID, ts, stamp)
	}
}

func TestUnit_EncodeAnnounceLegacy(t *testing.T) {
	raw := EncodeAnnounceAppData("alice")
	name, err := DisplayNameFromAppData(raw)
	if err != nil || name != "alice" {
		t.Fatalf("name=%q err=%v", name, err)
	}
}

func TestUnit_DeliveryMethodConstantsStable(t *testing.T) {
	if MethodDirect != 0x02 || MethodPropagated != 0x03 || MethodPaper != 0x05 {
		t.Fatal("delivery method assignments drifted")
	}
}
