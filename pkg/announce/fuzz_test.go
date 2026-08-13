// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package announce

import (
	"encoding/hex"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

// FuzzHandleAnnounce ensures announce wire parsing never panics and enforces
// length and hop invariants on known-hostile shapes.
func FuzzHandleAnnounce(f *testing.F) {
	id, err := identity.New()
	if err != nil {
		f.Fatal(err)
	}
	destHash := DestinationHash(id, "fuzzapp")
	ann, err := New(id, destHash, "fuzzapp", []byte("app"), false, &common.ReticulumConfig{})
	if err != nil {
		f.Fatal(err)
	}
	good, err := ann.CreatePacket()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(good)
	f.Add([]byte{})
	f.Add([]byte{0x01, 0x00})
	f.Add(append([]byte(nil), good[:min(len(good), 32)]...))
	mut := append([]byte(nil), good...)
	if len(mut) > 4 {
		mut[3] ^= 0xff
	}
	f.Add(mut)

	// Adversarial corpus seeds (truncated announce, bad hops).
	if trunc, err := hex.DecodeString("01000100010001000100010001000100010001000100010001000100010001000100010001000100"); err == nil {
		f.Add(trunc)
	}
	badHops := make([]byte, MinAnnouncePacketSizeNoRatchet)
	badHops[0] = PacketTypeAnnounce
	badHops[1] = 0xff
	f.Add(badHops)

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		err1 := ann.HandleAnnounce(data)
		err2 := ann.HandleAnnounce(data)
		_ = err2

		if len(data) < MinAnnouncePacketSizeNoRatchet {
			if err1 == nil {
				t.Fatal("truncated announce must error")
			}
			return
		}
		if len(data) >= HeaderSize && data[0]&HeaderPacketTypeMask == PacketTypeAnnounce && data[1] >= MaxHops {
			if err1 == nil {
				t.Fatal("announce with hop count at or above MaxHops must error")
			}
			if !strings.Contains(err1.Error(), "hop") {
				t.Fatalf("hop overflow error %q missing hop wording", err1.Error())
			}
		}
	})
}
