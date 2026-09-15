// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"encoding/binary"
	"errors"
)

var (
	ErrShortBuffer     = errors.New("rgosh: short buffer")
	ErrOversizedField  = errors.New("rgosh: oversized field")
	ErrInvalidStreamID = errors.New("rgosh: invalid stream id")
	ErrDecompressBomb  = errors.New("rgosh: decompressed chunk exceeds limit")
	ErrUnknownMsgType  = errors.New("rgosh: unknown message type")
)

// NoopMessage is a keepalive.
type NoopMessage struct {
	Compat bool
}

func (m *NoopMessage) Pack() ([]byte, error) { return nil, nil }
func (m *NoopMessage) Unpack([]byte) error   { return nil }
func (m *NoopMessage) GetType() uint16 {
	if m.Compat {
		return CompatNoop
	}
	return NativeNoop
}

// VersionMessage carries software and protocol version.
type VersionMessage struct {
	Compat          bool
	SoftwareVersion string
	ProtocolVersion int
	Capabilities    uint16
}

func (m *VersionMessage) GetType() uint16 {
	if m.Compat {
		return CompatVersion
	}
	return NativeVersion
}

func (m *VersionMessage) Pack() ([]byte, error) {
	if m.Compat {
		return packCompatVersion(m)
	}
	sw := m.SoftwareVersion
	if sw == "" {
		sw = DefaultSoftware
	}
	if len(sw) > MaxSoftwareLen {
		return nil, ErrOversizedField
	}
	pv := m.ProtocolVersion
	if pv == 0 {
		pv = ProtocolVersion
	}
	buf := make([]byte, 2+2+2+len(sw))
	binary.BigEndian.PutUint16(buf[0:2], uint16(pv))
	binary.BigEndian.PutUint16(buf[2:4], m.Capabilities)
	binary.BigEndian.PutUint16(buf[4:6], wireLenU16(len(sw)))
	copy(buf[6:], sw)
	return buf, nil
}

func (m *VersionMessage) Unpack(raw []byte) error {
	if m.Compat {
		return unpackCompatVersion(m, raw)
	}
	if len(raw) < 6 {
		return ErrShortBuffer
	}
	m.ProtocolVersion = int(binary.BigEndian.Uint16(raw[0:2]))
	m.Capabilities = binary.BigEndian.Uint16(raw[2:4])
	n := int(binary.BigEndian.Uint16(raw[4:6]))
	if n > MaxSoftwareLen || 6+n > len(raw) {
		return ErrOversizedField
	}
	m.SoftwareVersion = string(raw[6 : 6+n])
	return nil
}

// AuthOKMessage confirms identity allowlist pass (native only).
type AuthOKMessage struct{}

func (m *AuthOKMessage) Pack() ([]byte, error) { return nil, nil }
func (m *AuthOKMessage) Unpack([]byte) error   { return nil }
func (m *AuthOKMessage) GetType() uint16       { return NativeAuthOK }

// AuthDeniedMessage rejects the peer (native only).
type AuthDeniedMessage struct {
	Reason string
}

func (m *AuthDeniedMessage) GetType() uint16 { return NativeAuthDenied }

func (m *AuthDeniedMessage) Pack() ([]byte, error) {
	if len(m.Reason) > MaxErrorLen {
		return nil, ErrOversizedField
	}
	buf := make([]byte, 2+len(m.Reason))
	binary.BigEndian.PutUint16(buf[0:2], wireLenU16(len(m.Reason)))
	copy(buf[2:], m.Reason)
	return buf, nil
}

func (m *AuthDeniedMessage) Unpack(raw []byte) error {
	if len(raw) < 2 {
		return ErrShortBuffer
	}
	n := int(binary.BigEndian.Uint16(raw[0:2]))
	if n > MaxErrorLen || 2+n > len(raw) {
		return ErrOversizedField
	}
	m.Reason = string(raw[2 : 2+n])
	return nil
}

// WinSizeMessage carries terminal geometry.
type WinSizeMessage struct {
	Compat bool
	Rows   int
	Cols   int
	HPix   int
	VPix   int
}

func (m *WinSizeMessage) GetType() uint16 {
	if m.Compat {
		return CompatWinSize
	}
	return NativeWinSize
}

func (m *WinSizeMessage) Pack() ([]byte, error) {
	if m.Compat {
		return packCompatWinSize(m)
	}
	buf := make([]byte, 8)
	binary.BigEndian.PutUint16(buf[0:2], clampU16(m.Rows))
	binary.BigEndian.PutUint16(buf[2:4], clampU16(m.Cols))
	binary.BigEndian.PutUint16(buf[4:6], clampU16(m.HPix))
	binary.BigEndian.PutUint16(buf[6:8], clampU16(m.VPix))
	return buf, nil
}

func (m *WinSizeMessage) Unpack(raw []byte) error {
	if m.Compat {
		return unpackCompatWinSize(m, raw)
	}
	if len(raw) < 8 {
		return ErrShortBuffer
	}
	m.Rows = int(binary.BigEndian.Uint16(raw[0:2]))
	m.Cols = int(binary.BigEndian.Uint16(raw[2:4]))
	m.HPix = int(binary.BigEndian.Uint16(raw[4:6]))
	m.VPix = int(binary.BigEndian.Uint16(raw[6:8]))
	return nil
}

// ExecMessage requests remote command start.
type ExecMessage struct {
	Compat     bool
	Cmdline    []string
	PipeStdin  bool
	PipeStdout bool
	PipeStderr bool
	Term       string
	Rows       int
	Cols       int
	HPix       int
	VPix       int
}

func (m *ExecMessage) GetType() uint16 {
	if m.Compat {
		return CompatExec
	}
	return NativeExec
}

func (m *ExecMessage) Pack() ([]byte, error) {
	if m.Compat {
		return packCompatExec(m)
	}
	if len(m.Cmdline) > MaxArgvLen {
		return nil, ErrOversizedField
	}
	if len(m.Term) > MaxTermLen {
		return nil, ErrOversizedField
	}
	total := 0
	for _, a := range m.Cmdline {
		total += len(a)
		if total > MaxArgBytes {
			return nil, ErrOversizedField
		}
	}
	var flags byte
	if m.PipeStdin {
		flags |= 1
	}
	if m.PipeStdout {
		flags |= 2
	}
	if m.PipeStderr {
		flags |= 4
	}
	size := 1 + 8 + 2 + len(m.Term) + 2
	for _, a := range m.Cmdline {
		size += 2 + len(a)
	}
	buf := make([]byte, size)
	buf[0] = flags
	binary.BigEndian.PutUint16(buf[1:3], clampU16(m.Rows))
	binary.BigEndian.PutUint16(buf[3:5], clampU16(m.Cols))
	binary.BigEndian.PutUint16(buf[5:7], clampU16(m.HPix))
	binary.BigEndian.PutUint16(buf[7:9], clampU16(m.VPix))
	off := 9
	binary.BigEndian.PutUint16(buf[off:off+2], wireLenU16(len(m.Term)))
	off += 2
	copy(buf[off:], m.Term)
	off += len(m.Term)
	binary.BigEndian.PutUint16(buf[off:off+2], wireLenU16(len(m.Cmdline)))
	off += 2
	for _, a := range m.Cmdline {
		binary.BigEndian.PutUint16(buf[off:off+2], wireLenU16(len(a)))
		off += 2
		copy(buf[off:], a)
		off += len(a)
	}
	return buf, nil
}

func (m *ExecMessage) Unpack(raw []byte) error {
	if m.Compat {
		return unpackCompatExec(m, raw)
	}
	if len(raw) < 11 {
		return ErrShortBuffer
	}
	flags := raw[0]
	m.PipeStdin = flags&1 != 0
	m.PipeStdout = flags&2 != 0
	m.PipeStderr = flags&4 != 0
	m.Rows = int(binary.BigEndian.Uint16(raw[1:3]))
	m.Cols = int(binary.BigEndian.Uint16(raw[3:5]))
	m.HPix = int(binary.BigEndian.Uint16(raw[5:7]))
	m.VPix = int(binary.BigEndian.Uint16(raw[7:9]))
	off := 9
	termLen := int(binary.BigEndian.Uint16(raw[off : off+2]))
	off += 2
	if termLen > MaxTermLen || off+termLen > len(raw) {
		return ErrOversizedField
	}
	m.Term = string(raw[off : off+termLen])
	off += termLen
	if off+2 > len(raw) {
		return ErrShortBuffer
	}
	argc := int(binary.BigEndian.Uint16(raw[off : off+2]))
	off += 2
	if argc > MaxArgvLen {
		return ErrOversizedField
	}
	m.Cmdline = make([]string, 0, argc)
	total := 0
	for range argc {
		if off+2 > len(raw) {
			return ErrShortBuffer
		}
		n := int(binary.BigEndian.Uint16(raw[off : off+2]))
		off += 2
		if n < 0 || off+n > len(raw) {
			return ErrShortBuffer
		}
		total += n
		if total > MaxArgBytes {
			return ErrOversizedField
		}
		m.Cmdline = append(m.Cmdline, string(raw[off:off+n]))
		off += n
	}
	return nil
}

// StreamMessage carries stdin/stdout/stderr bytes.
type StreamMessage struct {
	Compat     bool
	StreamID   int
	Data       []byte
	EOF        bool
	Compressed bool
}

func (m *StreamMessage) GetType() uint16 {
	if m.Compat {
		return CompatStream
	}
	return NativeStream
}

func (m *StreamMessage) Pack() ([]byte, error) {
	if m.StreamID < 0 || m.StreamID > 0x3fff {
		return nil, ErrInvalidStreamID
	}
	header := uint16(m.StreamID & 0x3fff)
	if m.EOF {
		header |= 0x8000
	}
	if m.Compressed {
		header |= 0x4000
	}
	buf := make([]byte, 2+len(m.Data))
	binary.BigEndian.PutUint16(buf[0:2], header)
	copy(buf[2:], m.Data)
	return buf, nil
}

func (m *StreamMessage) Unpack(raw []byte) error {
	if len(raw) < 2 {
		return ErrShortBuffer
	}
	header := binary.BigEndian.Uint16(raw[0:2])
	m.StreamID = int(header & 0x3fff)
	m.EOF = header&0x8000 != 0
	m.Compressed = header&0x4000 != 0
	m.Data = append([]byte(nil), raw[2:]...)
	if m.Compressed {
		plain, err := decompressBounded(m.Data, MaxDecompressed)
		if err != nil {
			return err
		}
		m.Data = plain
		m.Compressed = false
	}
	if len(m.Data) > MaxStreamChunk {
		return ErrOversizedField
	}
	return nil
}

// ErrorMessage reports a protocol or session error.
type ErrorMessage struct {
	Compat bool
	Msg    string
	Fatal  bool
}

func (m *ErrorMessage) GetType() uint16 {
	if m.Compat {
		return CompatError
	}
	return NativeError
}

func (m *ErrorMessage) Pack() ([]byte, error) {
	if m.Compat {
		return packCompatError(m)
	}
	if len(m.Msg) > MaxErrorLen {
		return nil, ErrOversizedField
	}
	var flags byte
	if m.Fatal {
		flags = 1
	}
	buf := make([]byte, 1+2+len(m.Msg))
	buf[0] = flags
	binary.BigEndian.PutUint16(buf[1:3], wireLenU16(len(m.Msg)))
	copy(buf[3:], m.Msg)
	return buf, nil
}

func (m *ErrorMessage) Unpack(raw []byte) error {
	if m.Compat {
		return unpackCompatError(m, raw)
	}
	if len(raw) < 3 {
		return ErrShortBuffer
	}
	m.Fatal = raw[0]&1 != 0
	n := int(binary.BigEndian.Uint16(raw[1:3]))
	if n > MaxErrorLen || 3+n > len(raw) {
		return ErrOversizedField
	}
	m.Msg = string(raw[3 : 3+n])
	return nil
}

// ExitMessage reports remote process exit code.
type ExitMessage struct {
	Compat     bool
	ReturnCode int
}

func (m *ExitMessage) GetType() uint16 {
	if m.Compat {
		return CompatExit
	}
	return NativeExit
}

func (m *ExitMessage) Pack() ([]byte, error) {
	if m.Compat {
		return packCompatExit(m)
	}
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, packExitCode(m.ReturnCode))
	return buf, nil
}

func (m *ExitMessage) Unpack(raw []byte) error {
	if m.Compat {
		return unpackCompatExit(m, raw)
	}
	if len(raw) < 4 {
		return ErrShortBuffer
	}
	m.ReturnCode = unpackExitCode(binary.BigEndian.Uint32(raw[0:4]))
	return nil
}

// NewMessage constructs an empty message for the given type.
func NewMessage(msgType uint16) (interface {
	Pack() ([]byte, error)
	Unpack([]byte) error
	GetType() uint16
}, error) {
	compat := msgType&0xff00 == MsgMagicCompat<<8
	native := msgType&0xff00 == MsgMagicNative<<8
	if !compat && !native {
		return nil, ErrUnknownMsgType
	}
	code := msgType & 0x00ff
	switch code {
	case 0:
		return &NoopMessage{Compat: compat}, nil
	case 2:
		return &WinSizeMessage{Compat: compat}, nil
	case 3:
		return &ExecMessage{Compat: compat}, nil
	case 4:
		return &StreamMessage{Compat: compat}, nil
	case 5:
		return &VersionMessage{Compat: compat}, nil
	case 6:
		return &ErrorMessage{Compat: compat}, nil
	case 7:
		return &ExitMessage{Compat: compat}, nil
	case 8:
		if compat {
			return nil, ErrUnknownMsgType
		}
		return &AuthOKMessage{}, nil
	case 9:
		if compat {
			return nil, ErrUnknownMsgType
		}
		return &AuthDeniedMessage{}, nil
	default:
		return nil, ErrUnknownMsgType
	}
}

// UnpackMessage unpacks raw body for msgType.
func UnpackMessage(msgType uint16, raw []byte) (interface {
	Pack() ([]byte, error)
	Unpack([]byte) error
	GetType() uint16
}, error) {
	msg, err := NewMessage(msgType)
	if err != nil {
		return nil, err
	}
	if err := msg.Unpack(raw); err != nil {
		return nil, err
	}
	return msg, nil
}

func clampU16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 0xffff {
		return 0xffff
	}
	// #nosec G115 -- clamped to uint16 range
	return uint16(v)
}

func wireLenU16(n int) uint16 {
	// #nosec G115 -- caller enforces Max* limits before encoding
	return uint16(n)
}

func packExitCode(code int) uint32 {
	// #nosec G115 -- exit status wire format is signed 32-bit
	return uint32(int32(code))
}

func unpackExitCode(raw uint32) int {
	// #nosec G115 -- exit status wire format is signed 32-bit
	return int(int32(raw))
}
