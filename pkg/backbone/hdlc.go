// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package backbone

const (
	hdlcFlag    = 0x7E
	hdlcEsc     = 0x7D
	hdlcEscMask = 0x20
)

// HDLCDecoder incrementally parses HDLC-framed packets from a byte stream.
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
	for _, b := range buf {
		if b == hdlcFlag {
			if d.inFrame && len(d.data) > 0 {
				d.emit()
			}
			d.inFrame = !d.inFrame
			d.escape = false
			continue
		}
		if !d.inFrame {
			continue
		}
		if b == hdlcEsc {
			d.escape = true
			continue
		}
		if d.escape {
			b ^= hdlcEscMask
			d.escape = false
		}
		if len(d.data) >= d.maxFrame {
			d.data = d.data[:0]
			d.inFrame = false
			d.escape = false
			continue
		}
		d.data = append(d.data, b)
	}
}

func (d *HDLCDecoder) emit() {
	if d.onPacket == nil || len(d.data) == 0 {
		d.data = d.data[:0]
		return
	}
	out := append([]byte(nil), d.data...)
	d.data = d.data[:0]
	// Match Python BackboneClientInterface.check_frame_len (RNS 1.3.9).
	const headerMinSize = 19
	if len(out) <= headerMinSize {
		return
	}
	if d.mtu > 0 && len(out) > d.mtu {
		return
	}
	d.onPacket(out)
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
