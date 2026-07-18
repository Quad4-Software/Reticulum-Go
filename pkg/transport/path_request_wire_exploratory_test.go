// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"

	"quad4/reticulum-go/pkg/identity"
)

func TestParsePathRequestWireShapes(t *testing.T) {
	h := identity.TruncatedHashLength / 8
	dest := bytes.Repeat([]byte{0x11}, h)
	tag := bytes.Repeat([]byte{0x22}, h)
	tid := bytes.Repeat([]byte{0x33}, h)

	cases := []struct {
		name    string
		data    []byte
		ok      bool
		wantTID bool
		tagLen  int
	}{
		{"empty", nil, false, false, 0},
		{"short", dest[:8], false, false, 0},
		{"tagless", dest, false, false, 0},
		{"dest_tag", append(append([]byte{}, dest...), tag...), true, false, h},
		{"dest_short_tag", append(append([]byte{}, dest...), 0xAB), true, false, 1},
		{"dest_tid_tag", append(append(append([]byte{}, dest...), tid...), tag...), true, true, h},
		{"dest_tid_long_tag", append(append(append([]byte{}, dest...), tid...), bytes.Repeat([]byte{0x44}, h+4)...), true, true, h},
		{"exact_32_is_tag_not_tid", append(append([]byte{}, dest...), tag...), true, false, h},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, tidOut, tagOut, ok := parsePathRequestWire(tc.data)
			if ok != tc.ok {
				t.Fatalf("ok=%v want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if !bytes.Equal(d, dest) {
				t.Fatalf("dest mismatch")
			}
			if tc.wantTID != (len(tidOut) == h) {
				t.Fatalf("tid present=%v want %v", len(tidOut) == h, tc.wantTID)
			}
			if tc.wantTID && !bytes.Equal(tidOut, tid) {
				t.Fatalf("tid mismatch")
			}
			if len(tagOut) != tc.tagLen {
				t.Fatalf("tag len=%d want %d", len(tagOut), tc.tagLen)
			}
		})
	}
}

// FuzzParsePathRequestWireExploratory locks dest/TID/tag slicing rules.
func FuzzParsePathRequestWireExploratory(f *testing.F) {
	h := identity.TruncatedHashLength / 8
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0x01}, h))
	f.Add(bytes.Repeat([]byte{0x02}, h*2))
	f.Add(bytes.Repeat([]byte{0x03}, h*3))
	f.Add(bytes.Repeat([]byte{0x04}, h*3+8))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<12 {
			t.Skip()
		}
		dest, tid, tag, ok := parsePathRequestWire(data)
		if len(data) < h {
			if ok {
				t.Fatal("accepted short payload")
			}
			return
		}
		if len(data) == h {
			if ok {
				t.Fatal("accepted tagless exact-hash payload")
			}
			return
		}
		if !ok {
			return
		}
		if !bytes.Equal(dest, data[:h]) {
			t.Fatal("dest slice mismatch")
		}
		if len(data) > h*2 {
			if !bytes.Equal(tid, data[h:h*2]) {
				t.Fatal("tid slice mismatch")
			}
			wantTag := data[h*2:]
			if len(wantTag) > h {
				wantTag = wantTag[:h]
			}
			if !bytes.Equal(tag, wantTag) {
				t.Fatal("tag slice mismatch with TID form")
			}
			return
		}
		if len(tid) != 0 {
			t.Fatal("tid set on dest+tag form")
		}
		wantTag := data[h:]
		if len(wantTag) > h {
			wantTag = wantTag[:h]
		}
		if !bytes.Equal(tag, wantTag) {
			t.Fatal("tag slice mismatch on dest+tag form")
		}
	})
}
