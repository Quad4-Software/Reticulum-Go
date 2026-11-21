// SPDX-License-Identifier: LicenseRef-Reticulum
package proto_test

import (
	"bytes"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestMutationDestHashHasNoSpareCap(t *testing.T) {
	id := bytes.Repeat([]byte{0x22}, 16)
	h := proto.TelephonyHash(id)
	if len(h) != 16 {
		t.Fatalf("len %d", len(h))
	}
	if cap(h) != 16 {
		t.Fatalf("cap %d allows append into sha256 leftover", cap(h))
	}
	orig := append([]byte(nil), h...)
	_ = append(h, 0xAA, 0xBB)
	if !bytes.Equal(h, orig) {
		t.Fatal("append mutated dest hash")
	}
	h2 := proto.TelephonyHash(id)
	if !bytes.Equal(h2, orig) {
		t.Fatal("hash not stable")
	}
}

func TestMutationEmptySignallingRejected(t *testing.T) {
	if _, err := proto.PackSignalling(nil); err != proto.ErrEmptyPacket {
		t.Fatalf("empty pack: %v", err)
	}
	if _, err := proto.Unpack(nil); err != proto.ErrEmptyPacket {
		t.Fatalf("empty unpack: %v", err)
	}
}

func TestMutationUnknownKeyDoesNotInventSignals(t *testing.T) {
	pkt, err := proto.Unpack([]byte{0x81, 0x02, 0x91, 0x03})
	if err != proto.ErrMissingFields {
		t.Fatalf("unknown-only map: %v %+v", err, pkt)
	}
}
