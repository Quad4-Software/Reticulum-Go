// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package announce

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

// FuzzHandleAnnounce ensures announce wire parsing never panics on
// arbitrary frames. Signature failures and length errors are expected.
func FuzzHandleAnnounce(f *testing.F) {
	id, err := identity.New()
	if err != nil {
		f.Fatal(err)
	}
	destHash := make([]byte, 16)
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

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		_ = ann.HandleAnnounce(data)
	})
}
