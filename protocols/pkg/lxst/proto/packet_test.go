// SPDX-License-Identifier: LicenseRef-Reticulum
package proto_test

import (
	"bytes"
	"crypto/sha256"
	"os/exec"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/internal/lxsttest"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestSignallingRoundTrip(t *testing.T) {
	signals := []int{
		proto.StatusAvailable,
		proto.StatusRinging,
		proto.SignalPreferredProfile(proto.DefaultProfile),
		proto.SignalPreferredMode(proto.DefaultMode),
	}
	raw, err := proto.PackSignalling(signals)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := proto.Unpack(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt.Signals) != len(signals) {
		t.Fatalf("signal count %d want %d", len(pkt.Signals), len(signals))
	}
	for i, s := range signals {
		if pkt.Signals[i] != s {
			t.Fatalf("signal %d: got %d want %d", i, pkt.Signals[i], s)
		}
	}
}

func TestFrameRoundTrip(t *testing.T) {
	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	raw, err := proto.PackFrame(proto.CodecOpus, payload)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := proto.Unpack(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt.Frames) != 1 {
		t.Fatalf("frame count %d", len(pkt.Frames))
	}
	codec, body, err := proto.SplitFrame(pkt.Frames[0])
	if err != nil {
		t.Fatal(err)
	}
	if codec != proto.CodecOpus {
		t.Fatalf("codec %d", codec)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("payload mismatch")
	}
}

func TestPackFrameMatchesUmsgpackBin(t *testing.T) {
	raw, err := proto.PackFrame(proto.CodecOpus, []byte{9, 8, 7})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x81, 0x01, 0xc4, 0x04, 0x01, 0x09, 0x08, 0x07}
	if !bytes.Equal(raw, want) {
		t.Fatalf("wire %x want %x", raw, want)
	}
}

func TestStatusAvailableWire(t *testing.T) {
	raw, err := proto.PackSignalling([]int{proto.StatusAvailable})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x81, 0x00, 0x91, 0x03}
	if !bytes.Equal(raw, want) {
		t.Fatalf("wire %x want %x", raw, want)
	}
}

func TestPreferredProfileWire(t *testing.T) {
	raw, err := proto.PackSignalling([]int{proto.SignalPreferredProfile(proto.ProfileQualityMedium)})
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := proto.Unpack(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt.Signals) != 1 || !proto.IsPreferredProfile(pkt.Signals[0]) {
		t.Fatalf("signals %+v", pkt.Signals)
	}
	if proto.ProfileFromSignal(pkt.Signals[0]) != proto.ProfileQualityMedium {
		t.Fatalf("profile %d", proto.ProfileFromSignal(pkt.Signals[0]))
	}
}

func TestUnpackCapsSignalling(t *testing.T) {
	sigs := make([]int, 40)
	for i := range sigs {
		sigs[i] = proto.StatusAvailable
	}
	raw, err := proto.PackSignalling(sigs)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := proto.Unpack(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt.Signals) > 32 {
		t.Fatalf("signals %d", len(pkt.Signals))
	}
}

func TestTelephonyHash(t *testing.T) {
	idHash := make([]byte, 16)
	for i := range idHash {
		idHash[i] = byte(i + 1)
	}
	got := proto.TelephonyHash(idHash)
	nameSum := sha256.Sum256([]byte("lxst.telephony"))
	combined := make([]byte, 10+len(idHash))
	copy(combined, nameSum[:10])
	copy(combined[10:], idHash)
	final := sha256.Sum256(combined)
	want := final[:16]
	if !bytes.Equal(got, want) {
		t.Fatalf("hash %x want %x", got, want)
	}
}

func TestPythonUmsgpackInterop(t *testing.T) {
	py := lxsttest.Python(t)
	script := `
from RNS.vendor import umsgpack as mp
import sys
data = sys.stdin.buffer.read()
obj = mp.unpackb(data)
sys.stdout.buffer.write(mp.packb(obj))
`
	raw, err := proto.PackSignalling([]int{
		proto.StatusConnecting,
		proto.SignalPreferredProfile(proto.DefaultProfile),
		proto.SignalPreferredMode(proto.DefaultMode),
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(py, "-c", script)
	cmd.Stdin = bytes.NewReader(raw)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("python unpack: %v", err)
	}
	pkt, err := proto.Unpack(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt.Signals) != 3 || pkt.Signals[0] != proto.StatusConnecting {
		t.Fatalf("python round trip signals %+v", pkt.Signals)
	}

	frame, err := proto.PackFrame(proto.CodecOpus, []byte{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(py, "-c", script)
	cmd.Stdin = bytes.NewReader(frame)
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("python frame unpack: %v", err)
	}
	pkt, err = proto.Unpack(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt.Frames) != 1 || pkt.Frames[0][0] != proto.CodecOpus {
		t.Fatalf("python frame round trip %+v", pkt.Frames)
	}
}

func TestPythonPackMatchesGo(t *testing.T) {
	py := lxsttest.Python(t)
	script := `
from RNS.vendor import umsgpack as mp
import sys
kind = sys.argv[1]
if kind == "sig":
    sys.stdout.buffer.write(mp.packb({0: [3, 4, 319, 241]}))
elif kind == "frame":
    sys.stdout.buffer.write(mp.packb({1: bytes([1, 9, 8, 7])}))
`
	out, err := exec.Command(py, "-c", script, "sig").Output()
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := proto.Unpack(out)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{3, 4, 319, 241}
	if len(pkt.Signals) != len(want) {
		t.Fatalf("python signals %+v", pkt.Signals)
	}
	for i, s := range want {
		if pkt.Signals[i] != s {
			t.Fatalf("python signal %d got %d want %d", i, pkt.Signals[i], s)
		}
	}

	out, err = exec.Command(py, "-c", script, "frame").Output()
	if err != nil {
		t.Fatal(err)
	}
	pkt, err = proto.Unpack(out)
	if err != nil {
		t.Fatal(err)
	}
	codec, body, err := proto.SplitFrame(pkt.Frames[0])
	if err != nil {
		t.Fatal(err)
	}
	if codec != proto.CodecOpus || !bytes.Equal(body, []byte{9, 8, 7}) {
		t.Fatalf("python frame codec=%d body=%x", codec, body)
	}
}

func FuzzUnpack(f *testing.F) {
	sig, _ := proto.PackSignalling([]int{proto.StatusAvailable})
	frame, _ := proto.PackFrame(proto.CodecOpus, []byte{1, 2, 3})
	f.Add(sig)
	f.Add(frame)
	f.Add([]byte{0xff})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = proto.Unpack(data)
	})
}
