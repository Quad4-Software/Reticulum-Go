// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

// hdlcStreamDecoder incrementally parses HDLC-framed packets from a byte stream.
// Payload bytes are unescaped during assembly; onFrame receives the decoded body.
type hdlcStreamDecoder struct {
	mtu      int
	inFrame  bool
	escape   bool
	toggle   bool
	data     []byte
	maxFrame int
	onFrame  func([]byte)
}

func newHDLCStreamDecoder(mtu int, onFrame func([]byte)) *hdlcStreamDecoder {
	return newHDLCStreamDecoderOpts(mtu, false, onFrame)
}

// newHDLCToggleStreamDecoder uses PPP-style flag toggling, matching TCP read loops.
func newHDLCToggleStreamDecoder(mtu int, onFrame func([]byte)) *hdlcStreamDecoder {
	return newHDLCStreamDecoderOpts(mtu, true, onFrame)
}

func newHDLCStreamDecoderOpts(mtu int, toggle bool, onFrame func([]byte)) *hdlcStreamDecoder {
	maxFrame := 2*mtu + 32
	if maxFrame < 256 {
		maxFrame = 2048
	}
	return &hdlcStreamDecoder{
		mtu:      mtu,
		toggle:   toggle,
		maxFrame: maxFrame,
		data:     make([]byte, 0, mtu),
		onFrame:  onFrame,
	}
}

func (d *hdlcStreamDecoder) reset() {
	d.inFrame = false
	d.escape = false
	d.data = d.data[:0]
}

func (d *hdlcStreamDecoder) feed(buf []byte) {
	for _, b := range buf {
		d.feedByte(b)
	}
}

func (d *hdlcStreamDecoder) feedByte(b byte) {
	if b == HDLCFlag {
		if d.inFrame && len(d.data) > 0 {
			if d.onFrame != nil {
				d.onFrame(d.data)
			}
		}
		d.data = d.data[:0]
		if d.toggle {
			d.inFrame = !d.inFrame
		} else {
			d.inFrame = true
		}
		d.escape = false
		return
	}
	if !d.inFrame {
		return
	}
	if b == HDLCEsc {
		d.escape = true
		return
	}
	if d.escape {
		b ^= HDLCEscMask
		d.escape = false
	}
	if len(d.data) >= d.maxFrame {
		d.data = d.data[:0]
		d.inFrame = false
		d.escape = false
		return
	}
	d.data = append(d.data, b)
}
