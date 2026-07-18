// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"testing"
)

// FuzzDecodeAppData ensures the app_data parser is panic-free for arbitrary
// inputs. It does not assert successful decoding. Bad inputs must surface as

// a regular error.
func FuzzDecodeAppData(f *testing.F) {
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0xff}, StampSize+1))
	f.Add(bytes.Repeat([]byte{0x00}, 1024))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<16 {
			t.Skip()
		}
		_, _, _, err := DecodeAppData(raw)
		_ = err
	})
}

// FuzzDecodeInfo confirms the msgpack info parser survives arbitrary inputs.
func FuzzDecodeInfo(f *testing.F) {
	good, err := EncodeInfo(Info{
		Type:        "TCPInterface",
		Transport:   true,
		TransportID: bytes.Repeat([]byte{0xab}, 16),
		Name:        "fuzz",
		Latitude:    1.0,
		Longitude:   2.0,
		Height:      3.0,
	})
	if err == nil {
		f.Add(good)
	}
	f.Add([]byte{})
	f.Add([]byte{0x80})
	f.Add(bytes.Repeat([]byte{0xff}, 1024))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<16 {
			t.Skip()
		}
		_, err := DecodeInfo(raw)
		_ = err
	})
}

// FuzzStampValid ensures StampValid handles arbitrary stamp/workblock
// inputs without panicking, and rejects wrong-length stamps.
func FuzzStampValid(f *testing.F) {
	f.Add([]byte{}, []byte{}, 0)
	f.Add(bytes.Repeat([]byte{0x00}, StampSize), bytes.Repeat([]byte{0x00}, 256), 256)
	f.Add(bytes.Repeat([]byte{0xff}, StampSize), bytes.Repeat([]byte{0xff}, 256), -1)
	f.Add(bytes.Repeat([]byte{0xaa}, 16), bytes.Repeat([]byte{0xbb}, 64), 9999)

	f.Fuzz(func(t *testing.T, stamp, workblock []byte, target int) {
		if len(stamp) > 1<<10 || len(workblock) > 1<<14 {
			t.Skip()
		}
		ok := StampValid(stamp, target, workblock)
		if len(stamp) != StampSize && ok {
			t.Fatalf("StampValid accepted len=%d want reject unless %d", len(stamp), StampSize)
		}
	})
}

// FuzzEncodeDecodeInfoRoundTrip confirms EncodeInfo fields survive DecodeInfo.
func FuzzEncodeDecodeInfoRoundTrip(f *testing.F) {
	f.Add("TCPInterface", []byte{1, 2, 3, 4}, "node-a", true, int64(4242))
	f.Add("AutoInterface", bytes.Repeat([]byte{0xab}, 16), "", false, int64(0))

	f.Fuzz(func(t *testing.T, typ string, tid []byte, name string, transport bool, port int64) {
		if typ == "" || len(tid) == 0 || len(tid) > 64 || len(name) > 256 {
			t.Skip()
		}
		in := Info{
			Type:        typ,
			TransportID: tid,
			Name:        name,
			Transport:   transport,
			Latitude:    1.5,
			Longitude:   -2.25,
			Height:      10,
		}
		if port != 0 {
			in.Port = port
			in.HasPort = true
		}
		raw, err := EncodeInfo(in)
		if err != nil {
			t.Fatalf("EncodeInfo: %v", err)
		}
		out, err := DecodeInfo(raw)
		if err != nil {
			t.Fatalf("DecodeInfo: %v", err)
		}
		if out.Type != in.Type || out.Name != in.Name || out.Transport != in.Transport {
			t.Fatalf("string/bool fields drifted: %+v vs %+v", out, in)
		}
		if !bytes.Equal(out.TransportID, in.TransportID) {
			t.Fatalf("TransportID mismatch")
		}
		if out.Latitude != in.Latitude || out.Longitude != in.Longitude || out.Height != in.Height {
			t.Fatalf("geo mismatch got lat=%v lon=%v h=%v", out.Latitude, out.Longitude, out.Height)
		}
		if in.HasPort && (!out.HasPort || out.Port != in.Port) {
			t.Fatalf("port mismatch got HasPort=%v Port=%d want %d", out.HasPort, out.Port, in.Port)
		}
		if !in.HasPort && out.HasPort {
			t.Fatal("HasPort set unexpectedly")
		}
	})
}
