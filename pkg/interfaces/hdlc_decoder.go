// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

// hdlcStreamDecoder incrementally parses HDLC-framed packets from a byte stream.
// Payload bytes are unescaped during assembly, and onFrame receives the decoded body.
type hdlcStreamDecoder struct {
	mtu        int
	minPayload int
	inFrame    bool
	escape     bool
	toggle     bool
	data       []byte
	maxFrame   int
	onFrame    func([]byte)
}

func newHDLCStreamDecoder(mtu int, onFrame func([]byte)) *hdlcStreamDecoder {
	return newHDLCStreamDecoderOpts(mtu, false, 0, onFrame)
}

// newHDLCToggleStreamDecoder uses PPP-style flag toggling, matching TCP read loops.
func newHDLCToggleStreamDecoder(mtu int, onFrame func([]byte)) *hdlcStreamDecoder {
	return newHDLCStreamDecoderOpts(mtu, true, 0, onFrame)
}

// newTCPHDLCStreamDecoder matches Python TCPClientInterface.check_frame_len (RNS 1.3.9).
func newTCPHDLCStreamDecoder(mtu int, onFrame func([]byte)) *hdlcStreamDecoder {
	return newHDLCStreamDecoderOpts(mtu, true, reticulumHeaderMinSize, onFrame)
}

func newHDLCStreamDecoderOpts(mtu int, toggle bool, minPayload int, onFrame func([]byte)) *hdlcStreamDecoder {
	maxFrame := 2*mtu + 32
	if maxFrame < 256 {
		maxFrame = 2048
	}
	return &hdlcStreamDecoder{
		mtu:        mtu,
		minPayload: minPayload,
		toggle:     toggle,
		maxFrame:   maxFrame,
		data:       make([]byte, 0, mtu),
		onFrame:    onFrame,
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

// dropPartial clears an incomplete frame. Used when a serial inter-byte idle
// timeout expires so noise does not stick forever in the assembler.
func (d *hdlcStreamDecoder) dropPartial() bool {
	if !d.inFrame && len(d.data) == 0 && !d.escape {
		return false
	}
	had := d.inFrame || len(d.data) > 0 || d.escape
	d.reset()
	if !d.toggle {
		d.inFrame = false
	}
	return had
}

func (d *hdlcStreamDecoder) feedByte(b byte) {
	if b == HDLCFlag {
		if d.inFrame && len(d.data) > 0 {
			// maxFrame allows escaped assembly headroom; delivered payload
			// must still fit the interface MTU (matching KISS). TCP/Backbone
			// also reject frames at or below HEADER_MINSIZE (RNS 1.3.9).
			ok := d.onFrame != nil && (d.mtu <= 0 || len(d.data) <= d.mtu)
			if ok && d.minPayload > 0 && len(d.data) <= d.minPayload {
				ok = false
			}
			if ok {
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
	limit := d.maxFrame
	if d.mtu > 0 && d.mtu < limit {
		limit = d.mtu
	}
	if len(d.data) >= limit {
		d.data = d.data[:0]
		d.inFrame = false
		d.escape = false
		return
	}
	d.data = append(d.data, b)
}
