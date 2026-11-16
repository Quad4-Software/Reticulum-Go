// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

func TestMutation_UnpackStructural(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)
	msg, err := NewMessage(dst.Hash(), src.Hash(), []byte("t"), []byte("payload"), nil)
	if err != nil {
		t.Fatal(err)
	}
	base := mustPack(t, msg, src)

	mutations := []struct {
		name string
		fn   func([]byte) []byte
	}{
		{"truncate", func(b []byte) []byte {
			if len(b) < 4 {
				return b
			}
			return b[:len(b)/2]
		}},
		{"flip-sig", func(b []byte) []byte {
			out := append([]byte(nil), b...)
			off := 2 * DestinationLength
			if len(out) > off {
				out[off] ^= 0xff
			}
			return out
		}},
		{"flip-payload", func(b []byte) []byte {
			out := append([]byte(nil), b...)
			if len(out) > 0 {
				out[len(out)-1] ^= 0xaa
			}
			return out
		}},
		{"append-noise", func(b []byte) []byte {
			return append(append([]byte(nil), b...), 0x00, 0xff)
		}},
		{"zero-dest", func(b []byte) []byte {
			out := append([]byte(nil), b...)
			for i := 0; i < DestinationLength && i < len(out); i++ {
				out[i] = 0
			}
			return out
		}},
	}

	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			mut := m.fn(base)
			got, err := Unpack(mut, RecallSource)
			if m.name == "append-noise" || m.name == "truncate" || m.name == "flip-sig" || m.name == "flip-payload" {
				if err == nil && got != nil && got.SignatureValidated {
					t.Fatalf("%s: mutation still validated", m.name)
				}
				return
			}
			if err != nil {
				return
			}
			_ = got.TitleString()
			_ = got.ContentString()
			_ = got.Hash
		})
	}
}

func TestMutation_PaperURICorrupt(t *testing.T) {
	uri, err := PaperURI([]byte("mut"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"",
		"http://x",
		"lxm://",
		uri + "!!!!",
		uri[:len(uri)-1],
	}
	for _, c := range cases {
		_, _ = DecodePaperURI(c)
	}
}

func TestMutation_ContainerCorrupt(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)
	msg, err := NewMessage(dst.Hash(), src.Hash(), nil, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = mustPack(t, msg, src)
	base, err := PackContainer(msg)
	if err != nil {
		t.Fatal(err)
	}
	for _, mut := range [][]byte{
		nil,
		{0xff},
		base[:len(base)/2],
		append(append([]byte(nil), base...), 0x01),
	} {
		_, _, _ = UnpackContainer(mut, RecallSource)
	}
}

func TestMutation_AnnounceAppDataJunk(t *testing.T) {
	junk := [][]byte{
		nil,
		{},
		{0xff, 0x00},
		bytes.Repeat([]byte{0x92}, 64),
		[]byte("not-msgpack"),
	}
	for _, j := range junk {
		_, _ = DisplayNameFromAppData(j)
		_, _, _ = StampCostFromAppData(j)
		_, _, _ = CompressionSupportFromAppData(j)
		_ = PNAnnounceDataIsValid(j)
	}
}
