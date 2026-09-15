// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"encoding/binary"
	"math"
)

// RNode KISS command bytes match Python RNS.Interfaces.RNodeInterface.KISS
// (RNS 1.5.2). Unlike TCPInterface KISS, command bytes are not masked to 0x0F.
const (
	rnodeCmdUnknown    byte = 0xFE
	rnodeCmdData       byte = 0x00
	rnodeCmdFrequency  byte = 0x01
	rnodeCmdBandwidth  byte = 0x02
	rnodeCmdTXPower    byte = 0x03
	rnodeCmdSF         byte = 0x04
	rnodeCmdCR         byte = 0x05
	rnodeCmdRadioState byte = 0x06
	rnodeCmdRadioLock  byte = 0x07
	rnodeCmdDetect     byte = 0x08
	rnodeCmdLeave      byte = 0x0A
	rnodeCmdSTAlock    byte = 0x0B
	rnodeCmdLTAlock    byte = 0x0C
	rnodeCmdReady      byte = 0x0F
	rnodeCmdStatRX     byte = 0x21
	rnodeCmdStatTX     byte = 0x22
	rnodeCmdStatRSSI   byte = 0x23
	rnodeCmdStatSNR    byte = 0x24
	rnodeCmdStatCHTM   byte = 0x25
	rnodeCmdStatPhyPrm byte = 0x26
	rnodeCmdStatBat    byte = 0x27
	rnodeCmdStatCSMA   byte = 0x28
	rnodeCmdStatTemp   byte = 0x29
	rnodeCmdBlink      byte = 0x30
	rnodeCmdRandom     byte = 0x40
	rnodeCmdFBExt      byte = 0x41
	rnodeCmdFBRead     byte = 0x42
	rnodeCmdFBWrite    byte = 0x43
	rnodeCmdBTCtrl     byte = 0x46
	rnodeCmdPlatform   byte = 0x48
	rnodeCmdMCU        byte = 0x49
	rnodeCmdFWVersion  byte = 0x50
	rnodeCmdROMRead    byte = 0x51
	rnodeCmdReset      byte = 0x55
	rnodeCmdDispRead   byte = 0x66
	rnodeCmdInterfaces byte = 0x71
	rnodeCmdError      byte = 0x90
	rnodeCmdSelInt     byte = 0x1F

	rnodeDetectReq  byte = 0x73
	rnodeDetectResp byte = 0x46

	rnodeRadioStateOff byte = 0x00
	rnodeRadioStateOn  byte = 0x01
	rnodeRadioStateAsk byte = 0xFF

	rnodeErrorInitRadio    byte = 0x01
	rnodeErrorTXFailed     byte = 0x02
	rnodeErrorEEPROMLocked byte = 0x03
	rnodeErrorQueueFull    byte = 0x04
	rnodeErrorMemoryLow    byte = 0x05
	rnodeErrorModemTimeout byte = 0x06

	rnodePlatformAVR   byte = 0x90
	rnodePlatformESP32 byte = 0x80
	rnodePlatformNRF52 byte = 0x70

	rnodeSX127X byte = 0x00
	rnodeSX1276 byte = 0x01
	rnodeSX1278 byte = 0x02
	rnodeSX126X byte = 0x10
	rnodeSX1262 byte = 0x11
	rnodeSX128X byte = 0x20
	rnodeSX1280 byte = 0x21

	rnodeCmdInt0Data  byte = 0x00
	rnodeCmdInt1Data  byte = 0x10
	rnodeCmdInt2Data  byte = 0x20
	rnodeCmdInt3Data  byte = 0x70
	rnodeCmdInt4Data  byte = 0x75
	rnodeCmdInt5Data  byte = 0x90
	rnodeCmdInt6Data  byte = 0xA0
	rnodeCmdInt7Data  byte = 0xB0
	rnodeCmdInt8Data  byte = 0xC0
	rnodeCmdInt9Data  byte = 0xD0
	rnodeCmdInt10Data byte = 0xE0
	rnodeCmdInt11Data byte = 0xF0

	rnodeHWMTU                    = 508
	rnodeDefaultIFACSize          = 8
	rnodeFreqMin            int64 = 137000000
	rnodeFreqMax            int64 = 3000000000
	rnodeRSSIOffset               = 157
	rnodeCallsignMaxLen           = 32
	rnodeRequiredFWMaj            = 1
	rnodeRequiredFWMin            = 52
	rnodeMultiRequiredMaj         = 1
	rnodeMultiRequiredMin         = 74
	rnodeMaxSubInterfaces         = 11
	rnodeQSNRMinBase              = -9.0
	rnodeQSNRMax                  = 6.0
	rnodeQSNRStep                 = 2.0
	rnodeBatteryUnknown           = 0x00
	rnodeBatteryDischarging       = 0x01
	rnodeBatteryCharging          = 0x02
	rnodeBatteryCharged           = 0x03
)

var rnodeIntDataCmds = [...]byte{
	rnodeCmdInt0Data,
	rnodeCmdInt1Data,
	rnodeCmdInt2Data,
	rnodeCmdInt3Data,
	rnodeCmdInt4Data,
	rnodeCmdInt5Data,
	rnodeCmdInt6Data,
	rnodeCmdInt7Data,
	rnodeCmdInt8Data,
	rnodeCmdInt9Data,
	rnodeCmdInt10Data,
	rnodeCmdInt11Data,
}

// Wire helpers keep gosec G115 quiet where values are already validated to
// RNode protocol ranges before encoding or decoded from signed wire bytes.
func rnodeWireU32(v int64) uint32 {
	return uint32(v) // #nosec G115 -- frequency/bandwidth validated before encode
}

func rnodeWireU32Int(v int) uint32 {
	return uint32(v) // #nosec G115 -- bandwidth validated before encode
}

func rnodeWireByte(v int) byte {
	return byte(v) // #nosec G115 -- SF/CR/TXPower/index validated before encode
}

func rnodeWireTXPower(v int) byte {
	return byte(int8(v)) // #nosec G115 -- TX power validated to -9..37
}

func rnodeSNRFromWire(b byte) float64 {
	return float64(int8(b)) * 0.25 // #nosec G115 -- SNR is signed Q format on the wire
}

func rnodeTXPowerFromWire(b byte) int {
	return int(int8(b)) // #nosec G115 -- TX power is signed on the wire
}

func rnodeEscape(dst, data []byte) []byte {
	for _, b := range data {
		switch b {
		case KISSFesc:
			dst = append(dst, KISSFesc, KISSTFesc)
		case KISSFend:
			dst = append(dst, KISSFesc, KISSTFend)
		default:
			dst = append(dst, b)
		}
	}
	return dst
}

func appendRNodeFrame(dst []byte, cmd byte, payload []byte) []byte {
	dst = append(dst, KISSFend, cmd)
	dst = rnodeEscape(dst, payload)
	return append(dst, KISSFend)
}

func appendRNodeDataFrame(dst, payload []byte) []byte {
	return appendRNodeFrame(dst, rnodeCmdData, payload)
}

func appendRNodeSelIntFrame(dst []byte, index byte) []byte {
	return appendRNodeFrame(dst, rnodeCmdSelInt, []byte{index})
}

func appendRNodeU32Frame(dst []byte, cmd byte, v uint32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], v)
	return appendRNodeFrame(dst, cmd, buf[:])
}

func appendRNodeDetect(dst []byte) []byte {
	dst = appendRNodeFrame(dst, rnodeCmdDetect, []byte{rnodeDetectReq})
	dst = appendRNodeFrame(dst, rnodeCmdFWVersion, []byte{0x00})
	dst = appendRNodeFrame(dst, rnodeCmdPlatform, []byte{0x00})
	dst = appendRNodeFrame(dst, rnodeCmdMCU, []byte{0x00})
	return dst
}

func appendRNodeMultiDetect(dst []byte) []byte {
	dst = appendRNodeDetect(dst)
	return appendRNodeFrame(dst, rnodeCmdInterfaces, []byte{0x00})
}

func rnodeInterfaceTypeString(t byte) string {
	switch t {
	case rnodeSX126X, rnodeSX1262:
		return "SX126X"
	case rnodeSX127X, rnodeSX1276, rnodeSX1278:
		return "SX127X"
	case rnodeSX128X, rnodeSX1280:
		return "SX128X"
	default:
		return "SX127X"
	}
}

func rnodeIntDataIndex(cmd byte) (int, bool) {
	for i, c := range rnodeIntDataCmds {
		if c == cmd {
			return i, true
		}
	}
	return 0, false
}

func rnodeIsIntDataCmd(cmd byte) bool {
	_, ok := rnodeIntDataIndex(cmd)
	return ok
}

func rnodeComputeBitrate(sf, cr int, bandwidth int) float64 {
	if sf <= 0 || cr <= 0 || bandwidth <= 0 || sf > 63 {
		return 0
	}
	// Same formula as Python RNodeInterface.updateBitrate, with 2**sf via shift.
	return float64(sf) * ((4.0 / float64(cr)) / (float64(uint64(1)<<uint(sf)) / (float64(bandwidth) / 1000.0))) * 1000.0
}

func rnodeLinkQuality(sf int, snr float64) float64 {
	sfs := float64(sf - 7)
	qMin := rnodeQSNRMinBase - sfs*rnodeQSNRStep
	span := rnodeQSNRMax - qMin
	if span == 0 {
		return 0
	}
	q := ((snr - qMin) / span) * 100.0
	if q > 100 {
		q = 100
	}
	if q < 0 {
		q = 0
	}
	return math.Round(q*10) / 10
}

func rnodeFirmwareOK(maj, min, reqMaj, reqMin int) bool {
	if maj > reqMaj {
		return true
	}
	if maj < reqMaj {
		return false
	}
	return min >= reqMin
}

// rnodeCmdDecoder parses RNode KISS command streams (full command byte).
// onEvent receives a view of the assembler buffer that is reused after the
// callback returns. Callers that retain the frame must copy it. Transport
// HandlePacket already copies, matching hdlcStreamDecoder.
type rnodeCmdDecoder struct {
	mtu     int
	inFrame bool
	escape  bool
	command byte
	haveCmd bool
	data    []byte
	onEvent func(cmd byte, payload []byte)
}

func newRNodeCmdDecoder(mtu int, onEvent func(byte, []byte)) *rnodeCmdDecoder {
	if mtu <= 0 {
		mtu = rnodeHWMTU
	}
	return &rnodeCmdDecoder{
		mtu:     mtu,
		command: rnodeCmdUnknown,
		data:    make([]byte, 0, mtu),
		onEvent: onEvent,
	}
}

func (d *rnodeCmdDecoder) reset() {
	d.inFrame = false
	d.escape = false
	d.haveCmd = false
	d.command = rnodeCmdUnknown
	d.data = d.data[:0]
}

func (d *rnodeCmdDecoder) feed(buf []byte) {
	for _, b := range buf {
		d.feedByte(b)
	}
}

func (d *rnodeCmdDecoder) feedByte(b byte) {
	if d.inFrame && b == KISSFend && d.haveCmd {
		cmd := d.command
		payload := d.data
		if d.onEvent != nil {
			d.onEvent(cmd, payload)
		}
		d.reset()
		return
	}
	if b == KISSFend {
		d.inFrame = true
		d.command = rnodeCmdUnknown
		d.haveCmd = false
		d.data = d.data[:0]
		d.escape = false
		return
	}
	if !d.inFrame {
		return
	}
	if !d.haveCmd {
		d.command = b
		d.haveCmd = true
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
