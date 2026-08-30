// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import "testing"

func TestKISSStreamDecoderResetClearsPartialFrame(t *testing.T) {
	var got int
	d := newKISSStreamDecoder(64, func(b []byte) { got++ })
	d.feed([]byte{KISSFend, KISSCmdData, 0x01, 0x02})
	if !d.inFrame || len(d.data) != 2 {
		t.Fatalf("partial state inFrame=%v len=%d", d.inFrame, len(d.data))
	}
	d.reset()
	if d.inFrame || d.escape || d.haveCmd || len(d.data) != 0 || d.command != kissCmdUnknown {
		t.Fatalf("reset incomplete: %+v", *d)
	}
	d.feed([]byte{KISSFend, KISSCmdData, 0x03, KISSFend})
	if got != 1 {
		t.Fatalf("frames=%d want 1 after reset", got)
	}
}
