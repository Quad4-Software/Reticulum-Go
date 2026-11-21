// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import "bytes"

// kissCmdUnknown matches Python RNS.Interfaces.TCPInterface.KISS.CMD_UNKNOWN.
const kissCmdUnknown byte = 0xFE

// kissStreamDecoder incrementally parses KISS frames matching Python
// TCPInterface / I2PInterface: FEND, command nibble, escaped payload, FEND.
// Only CMD_DATA frames are delivered. The command byte is stripped.
type kissStreamDecoder struct {
	mtu     int
	inFrame bool
	escape  bool
	haveCmd bool
	command byte
	data    []byte
	onFrame func([]byte)
}

func newKISSStreamDecoder(mtu int, onFrame func([]byte)) *kissStreamDecoder {
	if mtu <= 0 {
		mtu = DefaultMTU
	}
	return &kissStreamDecoder{
		mtu:     mtu,
		command: kissCmdUnknown,
		data:    make([]byte, 0, mtu),
		onFrame: onFrame,
	}
}

func (d *kissStreamDecoder) reset() {
	d.inFrame = false
	d.escape = false
	d.haveCmd = false
	d.command = kissCmdUnknown
	d.data = d.data[:0]
}

func (d *kissStreamDecoder) feed(buf []byte) {
	for len(buf) > 0 {
		if d.escape {
			d.feedByte(buf[0])
			buf = buf[1:]
			continue
		}
		if !(d.inFrame && d.haveCmd && d.command == KISSCmdData) {
			d.feedByte(buf[0])
			buf = buf[1:]
			continue
		}
		iFend := bytes.IndexByte(buf, KISSFend)
		iEsc := bytes.IndexByte(buf, KISSFesc)
		next := len(buf)
		if iFend >= 0 && iFend < next {
			next = iFend
		}
		if iEsc >= 0 && iEsc < next {
			next = iEsc
		}
		if next > 0 {
			if len(d.data)+next > d.mtu {
				// Same bound as feedByte: a frame past MTU is dropped whole.
				d.reset()
				return
			}
			d.data = append(d.data, buf[:next]...)
			buf = buf[next:]
			continue
		}
		d.feedByte(buf[0])
		buf = buf[1:]
	}
}

func (d *kissStreamDecoder) feedByte(b byte) {
	if d.inFrame && b == KISSFend && d.haveCmd && d.command == KISSCmdData {
		d.inFrame = false
		if d.onFrame != nil {
			d.onFrame(d.data)
		}
		d.data = d.data[:0]
		d.escape = false
		d.haveCmd = false
		d.command = kissCmdUnknown
		return
	}
	if b == KISSFend {
		d.inFrame = true
		d.command = kissCmdUnknown
		d.haveCmd = false
		d.data = d.data[:0]
		d.escape = false
		return
	}
	if !d.inFrame {
		return
	}
	if !d.haveCmd {
		d.command = b & 0x0F
		d.haveCmd = true
		return
	}
	if d.command != KISSCmdData {
		return
	}
	if len(d.data) >= d.mtu {
		// Match HDLC: drop the whole frame instead of delivering a truncated payload.
		d.reset()
		return
	}
	if b == KISSFesc {
		d.escape = true
		return
	}
	if d.escape {
		switch b {
		case KISSTFend:
			b = KISSFend
		case KISSTFesc:
			b = KISSFesc
		}
		d.escape = false
	}
	d.data = append(d.data, b)
}

// appendFrameKISS appends a complete KISS data frame to dst.
func appendFrameKISS(dst []byte, payload []byte) []byte {
	dst = append(dst, KISSFend, KISSCmdData)
	for _, b := range payload {
		switch b {
		case KISSFend:
			dst = append(dst, KISSFesc, KISSTFend)
		case KISSFesc:
			dst = append(dst, KISSFesc, KISSTFesc)
		default:
			dst = append(dst, b)
		}
	}
	return append(dst, KISSFend)
}
