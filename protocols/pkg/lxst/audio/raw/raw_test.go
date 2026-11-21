// SPDX-License-Identifier: LicenseRef-Reticulum
package raw_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/raw"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestRawRoundTrip(t *testing.T) {
	c := raw.New(1, 8)
	in := []int16{0, 1000, -1000, 32000, -32000, 12, -12, 7}
	enc, err := c.Encode(in)
	if err != nil {
		t.Fatal(err)
	}
	if enc[0]>>6 != raw.Bitdepth16 {
		t.Fatalf("header bitdepth %x", enc[0])
	}
	if enc[0]&0x3f != 0 {
		t.Fatalf("header %x", enc[0])
	}
	out, err := c.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) {
		t.Fatalf("len %d", len(out))
	}
	for i := range in {
		d := int(out[i]) - int(in[i])
		if d < 0 {
			d = -d
		}
		if d > 16 {
			t.Fatalf("sample %d in %d out %d", i, in[i], out[i])
		}
	}
}

func TestRawStereoHeader(t *testing.T) {
	c := raw.New(2, 4)
	enc, err := c.Encode([]int16{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if enc[0]&0x3f != 1 {
		t.Fatalf("channels header %x", enc[0])
	}
	if proto.CodecRaw != 0x00 {
		t.Fatal("codec byte")
	}
}

func TestRawClosed(t *testing.T) {
	c := raw.New(1, 4)
	_ = c.Close()
	if _, err := c.Encode([]int16{1}); err == nil {
		t.Fatal("expected closed")
	}
}
