// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import "bytes"

// hdlcStreamDecoder incrementally parses HDLC-framed packets from a byte stream.
// Payload bytes are unescaped during assembly, and onFrame receives the decoded body.
// The slice passed to onFrame is reused after onFrame returns. Callers that
// retain the frame must copy it.
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
	for len(buf) > 0 {
		if d.escape {
			d.feedByte(buf[0])
			buf = buf[1:]
			continue
		}
		if !d.inFrame {
			i := bytes.IndexByte(buf, HDLCFlag)
			if i < 0 {
				return
			}
			d.feedByte(buf[i])
			buf = buf[i+1:]
			continue
		}
		iFlag := bytes.IndexByte(buf, HDLCFlag)
		iEsc := bytes.IndexByte(buf, HDLCEsc)
		next := len(buf)
		if iFlag >= 0 && iFlag < next {
			next = iFlag
		}
		if iEsc >= 0 && iEsc < next {
			next = iEsc
		}
		if next > 0 {
			d.appendRun(buf[:next])
			buf = buf[next:]
			continue
		}
		d.feedByte(buf[0])
		buf = buf[1:]
	}
}

func (d *hdlcStreamDecoder) frameLimit() int {
	limit := d.maxFrame
	if d.mtu > 0 && d.mtu < limit {
		limit = d.mtu
	}
	return limit
}

func (d *hdlcStreamDecoder) appendRun(run []byte) {
	limit := d.frameLimit()
	room := limit - len(d.data)
	if room <= 0 {
		d.data = d.data[:0]
		d.inFrame = false
		d.escape = false
		return
	}
	if len(run) > room {
		d.data = d.data[:0]
		d.inFrame = false
		d.escape = false
		return
	}
	d.data = append(d.data, run...)
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

func (d *hdlcStreamDecoder) emitFrame() {
	ok := d.onFrame != nil && (d.mtu <= 0 || len(d.data) <= d.mtu)
	if ok && d.minPayload > 0 && len(d.data) <= d.minPayload {
		ok = false
	}
	if ok && len(d.data) > 0 {
		d.onFrame(d.data)
	}
	d.data = d.data[:0]
}

func (d *hdlcStreamDecoder) feedByte(b byte) {
	if b == HDLCFlag {
		if d.inFrame && len(d.data) > 0 {
			d.emitFrame()
		} else {
			d.data = d.data[:0]
		}
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
	if len(d.data) >= d.frameLimit() {
		d.data = d.data[:0]
		d.inFrame = false
		d.escape = false
		return
	}
	d.data = append(d.data, b)
}
