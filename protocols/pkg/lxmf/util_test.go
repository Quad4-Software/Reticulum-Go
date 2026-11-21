// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"testing"
)

func TestHexHash(t *testing.T) {
	if got := HexHash(nil); got != "<empty>" {
		t.Errorf("nil: %q", got)
	}
	h := []byte{0xde, 0xad}
	if got := HexHash(h); got != "dead" {
		t.Errorf("got %q", got)
	}
}

func TestPrettyHexRep(t *testing.T) {
	if got := PrettyHexRep([]byte{0x01}); got != "<01>" {
		t.Errorf("got %q", got)
	}
}

func TestMessageStoreFilenameRoundTrip(t *testing.T) {
	tid := bytes.Repeat([]byte{0x42}, 32)
	const recv = 1700009999.125
	const stampVal int64 = 42
	name := MessageStoreFilename(tid, recv, stampVal)
	gotTid, gotRecv, gotStamp, err := ParseMessageStoreFilename(name)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !bytes.Equal(gotTid, tid) {
		t.Error("transient id mismatch")
	}
	if gotRecv != recv {
		t.Errorf("recv: %g vs %g", gotRecv, recv)
	}
	if gotStamp != stampVal {
		t.Errorf("stamp: %d vs %d", gotStamp, stampVal)
	}
}

func TestParseMessageStoreFilename_Errors(t *testing.T) {
	_, _, _, err := ParseMessageStoreFilename("bad")
	if err == nil {
		t.Fatal("expected error")
	}
	_, _, _, err = ParseMessageStoreFilename("zzzz_abcd_1")
	if err == nil {
		t.Fatal("expected hex error")
	}
}
