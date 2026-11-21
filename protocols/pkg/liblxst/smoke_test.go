// SPDX-License-Identifier: LicenseRef-Reticulum
package liblxst_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/liblxst"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestLXSTCodecRoundTrip(t *testing.T) {
	signals := []int{
		proto.StatusAvailable,
		liblxst.SignalPreferredProfile(proto.ProfileQualityMedium),
		liblxst.SignalPreferredMode(proto.ModeFullDuplex),
	}
	data, code := liblxst.PackSignalling(signals)
	if code != liblxst.OK {
		t.Fatal(code, liblxst.LastError())
	}
	handle, code := liblxst.Unpack(data)
	if code != liblxst.OK {
		t.Fatal(code)
	}
	defer liblxst.PacketDestroy(handle)

	n, code := liblxst.PacketSignalCount(handle)
	if code != liblxst.OK || n != len(signals) {
		t.Fatalf("signal count %d code %d", n, code)
	}
	for i, want := range signals {
		got, code := liblxst.PacketSignalAt(handle, i)
		if code != liblxst.OK || got != want {
			t.Fatalf("signal[%d] got %d want %d", i, got, want)
		}
	}
}

func TestLXSTFrameRoundTrip(t *testing.T) {
	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	data, code := liblxst.PackFrame(proto.CodecOpus, payload)
	if code != liblxst.OK {
		t.Fatal(code)
	}
	handle, code := liblxst.Unpack(data)
	if code != liblxst.OK {
		t.Fatal(code)
	}
	defer liblxst.PacketDestroy(handle)

	n, code := liblxst.PacketFrameCount(handle)
	if code != liblxst.OK || n != 1 {
		t.Fatalf("frame count %d", n)
	}
	frame, code := liblxst.PacketFrameAt(handle, 0)
	if code != liblxst.OK {
		t.Fatal(code)
	}
	codec, gotPayload, code := liblxst.SplitFrame(frame)
	if code != liblxst.OK {
		t.Fatal(code)
	}
	if codec != proto.CodecOpus {
		t.Fatalf("codec %d", codec)
	}
	if string(gotPayload) != string(payload) {
		t.Fatalf("payload %x", gotPayload)
	}
}

func TestLXSTTelephonyHash(t *testing.T) {
	id := make([]byte, proto.IdentityHashLen)
	for i := range id {
		id[i] = byte(i + 1)
	}
	hash, code := liblxst.TelephonyHash(id)
	if code != liblxst.OK || len(hash) != proto.DestHashLen {
		t.Fatalf("hash len %d code %d", len(hash), code)
	}
	want := proto.TelephonyHash(id)
	if string(hash) != string(want) {
		t.Fatalf("hash mismatch")
	}
}
