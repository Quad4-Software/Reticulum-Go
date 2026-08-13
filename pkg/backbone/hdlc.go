// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package backbone

import "bytes"

const (
	hdlcFlag    = 0x7E
	hdlcEsc     = 0x7D
	hdlcEscMask = 0x20
)

// HDLCDecoder incrementally parses HDLC-framed packets from a byte stream.
// onPacket receives a view of the assembler buffer that is reused after the
// callback returns. Callers that retain the frame must copy it.
type HDLCDecoder struct {
	mtu      int
	inFrame  bool
	escape   bool
	data     []byte
	maxFrame int
	onPacket func([]byte)
}

func NewHDLCDecoder(mtu int, onPacket func([]byte)) *HDLCDecoder {
	maxFrame := 2*mtu + 32
	if maxFrame < 256 {
		maxFrame = 2048
	}
	return &HDLCDecoder{
		mtu:      mtu,
		maxFrame: maxFrame,
		data:     make([]byte, 0, mtu),
		onPacket: onPacket,
	}
}

func (d *HDLCDecoder) Feed(buf []byte) {
	for len(buf) > 0 {
		if d.escape {
			d.feedByte(buf[0])
			buf = buf[1:]
			continue
		}
		if !d.inFrame {
			i := bytes.IndexByte(buf, hdlcFlag)
			if i < 0 {
				return
			}
			d.feedByte(buf[i])
			buf = buf[i+1:]
			continue
		}
		iFlag := bytes.IndexByte(buf, hdlcFlag)
		iEsc := bytes.IndexByte(buf, hdlcEsc)
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

func (d *HDLCDecoder) appendRun(run []byte) {
	room := d.maxFrame - len(d.data)
	if room <= 0 || len(run) > room {
		d.data = d.data[:0]
		d.inFrame = false
		d.escape = false
		return
	}
	d.data = append(d.data, run...)
}

func (d *HDLCDecoder) feedByte(b byte) {
	if b == hdlcFlag {
		if d.inFrame && len(d.data) > 0 {
			d.emit()
		}
		d.inFrame = !d.inFrame
		d.escape = false
		return
	}
	if !d.inFrame {
		return
	}
	if b == hdlcEsc {
		d.escape = true
		return
	}
	if d.escape {
		b ^= hdlcEscMask
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

func (d *HDLCDecoder) emit() {
	if d.onPacket == nil || len(d.data) == 0 {
		d.data = d.data[:0]
		return
	}
	// Match Python BackboneClientInterface.check_frame_len (RNS 1.3.9).
	const headerMinSize = 19
	if len(d.data) <= headerMinSize || (d.mtu > 0 && len(d.data) > d.mtu) {
		d.data = d.data[:0]
		return
	}
	d.onPacket(d.data)
	d.data = d.data[:0]
}

func (d *HDLCDecoder) Reset() {
	d.inFrame = false
	d.escape = false
	d.data = d.data[:0]
}

func escapeHDLC(data []byte) []byte {
	need := len(data)
	for _, b := range data {
		if b == hdlcFlag || b == hdlcEsc {
			need++
		}
	}
	return appendEscapeHDLC(make([]byte, 0, need), data)
}

func appendEscapeHDLC(dst, data []byte) []byte {
	for _, b := range data {
		if b == hdlcFlag || b == hdlcEsc {
			dst = append(dst, hdlcEsc, b^hdlcEscMask)
		} else {
			dst = append(dst, b)
		}
	}
	return dst
}

// appendFrameHDLC appends a complete HDLC frame to dst.
func appendFrameHDLC(dst, payload []byte) []byte {
	dst = append(dst, hdlcFlag)
	dst = appendEscapeHDLC(dst, payload)
	return append(dst, hdlcFlag)
}

func unescapeHDLC(data []byte) []byte {
	out := make([]byte, 0, len(data))
	escape := false
	for _, b := range data {
		if escape {
			out = append(out, b^hdlcEscMask)
			escape = false
			continue
		}
		if b == hdlcEsc {
			escape = true
			continue
		}
		out = append(out, b)
	}
	return out
}

func frameHDLC(payload []byte) []byte {
	return appendFrameHDLC(make([]byte, 0, len(payload)*2+2), payload)
}
