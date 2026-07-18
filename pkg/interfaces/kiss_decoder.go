// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

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

func (d *kissStreamDecoder) feed(buf []byte) {
	for _, b := range buf {
		d.feedByte(b)
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
