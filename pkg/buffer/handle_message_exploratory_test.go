// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package buffer

import (
	"io"
	"testing"

	"quad4/reticulum-go/pkg/channel"
	"quad4/reticulum-go/pkg/transport"
)

func TestHandleMessageCorruptCompressedStillHonorsEOF(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	reader := NewRawChannelReader(3, ch)

	msg := &StreamDataMessage{
		StreamID:   3,
		Data:       []byte("not-bz2"),
		EOF:        true,
		Compressed: true,
	}
	if !reader.HandleMessage(msg) {
		t.Fatal("HandleMessage returned false")
	}
	if reader.buffer.Len() != 0 {
		t.Fatalf("buffer len=%d want 0 after corrupt compress", reader.buffer.Len())
	}
	if !reader.eof {
		t.Fatal("EOF not set after corrupt compressed final chunk")
	}
	n, err := reader.Read(make([]byte, 4))
	if n != 0 || err != io.EOF {
		t.Fatalf("Read: n=%d err=%v want EOF", n, err)
	}
}

// FuzzHandleMessageEOFExploratory feeds packed StreamDataMessage bodies through
// HandleMessage. When the EOF header bit is set the reader must reach eof
// regardless of compression success.
func FuzzHandleMessageEOFExploratory(f *testing.F) {
	plain := &StreamDataMessage{StreamID: 1, Data: []byte("ok"), EOF: true}
	if raw, err := plain.Pack(); err == nil {
		f.Add(raw)
	}
	bad := &StreamDataMessage{
		StreamID:   1,
		Data:       []byte{0xff, 0x00, 0x01},
		EOF:        true,
		Compressed: true,
	}
	if raw, err := bad.Pack(); err == nil {
		f.Add(raw)
	}
	f.Add([]byte{})
	f.Add([]byte{0x00})

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<16 {
			t.Skip()
		}
		var msg StreamDataMessage
		if err := msg.Unpack(raw); err != nil {
			return
		}
		msg.StreamID = 1
		link := &mockLink{status: transport.StatusActive}
		ch := channel.NewChannel(link)
		reader := NewRawChannelReader(1, ch)
		_ = reader.HandleMessage(&msg)
		if msg.EOF && !reader.eof {
			t.Fatalf("EOF bit set but reader.eof=false (compressed=%v dataLen=%d)",
				msg.Compressed, len(msg.Data))
		}
	})
}
