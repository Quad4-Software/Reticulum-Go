// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !haiku

package protect

import (
	"bytes"
	"testing"
	"time"
)

func FuzzParseMode(f *testing.F) {
	f.Add("off")
	f.Add("detect")
	f.Add("prevent")
	f.Add("ids")
	f.Add("ips")
	f.Add("")
	f.Add("@@@")
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = ParseMode(s)
	})
}

func FuzzPeekPacketClass(f *testing.F) {
	f.Add([]byte{0x01, 0x00})
	f.Add([]byte{0x02, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = PeekPacketClass(data)
	})
}

func FuzzAdmitPacket(f *testing.F) {
	f.Add("udp0", 64)
	f.Add("", 0)
	f.Add("x", 1<<20)
	f.Fuzz(func(t *testing.T, iface string, nbytes int) {
		if nbytes < 0 {
			nbytes = -nbytes
		}
		var buf bytes.Buffer
		e := New(Options{Mode: ModePrevent, MaxPPS: 10, MaxBPS: 1024, WarnWriter: &buf, WarnInterval: time.Hour, DisableAdaptive: true, DisableCoolDown: true})
		_ = e.AdmitPacket(iface, nbytes)
		d, rel := e.AdmitCrypto(iface)
		if d.Allow {
			rel()
		}
		d, rel = e.AdmitHandshake(iface)
		if d.Allow {
			rel()
		}
	})
}
