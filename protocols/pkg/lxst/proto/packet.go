// SPDX-License-Identifier: LicenseRef-Reticulum
package proto

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack/msgpcode"
)

var unpackReaderPool = sync.Pool{
	New: func() any { return bytes.NewReader(nil) },
}

var (
	ErrEmptyPacket    = errors.New("empty lxst packet")
	ErrMissingFields  = errors.New("lxst packet has no signalling or frames")
	ErrPacketTooLarge = errors.New("lxst packet exceeds size limits")
)

const (
	maxMapKeys = 4
	packBufCap = 128
)

type Packet struct {
	Signals []int
	Frames  [][]byte
}

func PackSignalling(signals []int) ([]byte, error) {
	if len(signals) == 0 {
		return nil, ErrEmptyPacket
	}
	if len(signals) > MaxSignals {
		signals = signals[:MaxSignals]
	}
	return marshalCompact(map[int][]int{FieldSignalling: signals})
}

func PackFrame(codec byte, payload []byte) ([]byte, error) {
	return PackFrameInto(nil, codec, payload)
}

func PackFrameInto(dst []byte, codec byte, payload []byte) ([]byte, error) {
	if len(payload) > MaxFrameBytes {
		payload = payload[:MaxFrameBytes]
	}
	n := 1 + len(payload)
	hdr := 2
	if n > 255 {
		hdr = 3
	}
	need := 2 + hdr + n
	if cap(dst) < need {
		dst = make([]byte, need)
	} else {
		dst = dst[:need]
	}
	dst[0] = 0x81
	dst[1] = byte(FieldFrames)
	off := 2
	if n <= 255 {
		dst[off] = 0xc4
		dst[off+1] = byte(n)
		off += 2
	} else {
		if n < 0 || n > 0xffff {
			return nil, ErrPacketTooLarge
		}
		var lenBuf [2]byte
		binary.BigEndian.PutUint16(lenBuf[:], uint16(n)) // #nosec G115 -- n checked above
		dst[off] = 0xc5
		dst[off+1] = lenBuf[0]
		dst[off+2] = lenBuf[1]
		off += 3
	}
	dst[off] = codec
	copy(dst[off+1:], payload)
	return dst, nil
}

func Unpack(data []byte) (Packet, error) {
	if len(data) == 0 {
		return Packet{}, ErrEmptyPacket
	}
	if len(data) > MaxUnpackBytes {
		return Packet{}, ErrPacketTooLarge
	}
	r := unpackReaderPool.Get().(*bytes.Reader)
	r.Reset(data)
	dec := msgpack.GetDecoder()
	dec.Reset(r)
	dec.SetDecodeDepthLimit(8)
	pkt, err := unpackMap(dec)
	dec.SetDecodeDepthLimit(0)
	msgpack.PutDecoder(dec)
	r.Reset(nil)
	unpackReaderPool.Put(r)
	return pkt, err
}

func unpackMap(dec *msgpack.Decoder) (Packet, error) {
	n, err := dec.DecodeMapLen()
	if err != nil {
		return Packet{}, err
	}
	if n < 1 || n > maxMapKeys {
		if n == 0 {
			return Packet{}, ErrMissingFields
		}
		return Packet{}, ErrPacketTooLarge
	}
	pkt := Packet{}
	for range n {
		key, ok, err := decodeIntOrSkip(dec)
		if err != nil {
			return Packet{}, err
		}
		if !ok {
			if err := dec.Skip(); err != nil {
				return Packet{}, err
			}
			continue
		}
		switch key {
		case FieldSignalling:
			pkt.Signals, err = decodeSignals(dec)
		case FieldFrames:
			pkt.Frames, err = decodeFrames(dec)
		default:
			err = dec.Skip()
		}
		if err != nil {
			return Packet{}, err
		}
	}
	if len(pkt.Signals) == 0 && len(pkt.Frames) == 0 {
		return Packet{}, ErrMissingFields
	}
	return pkt, nil
}

func SplitFrame(frame []byte) (codec byte, payload []byte, err error) {
	if len(frame) < 1 {
		return 0, nil, fmt.Errorf("media frame too short")
	}
	return frame[0], frame[1:], nil
}

func DestHash(identityHash []byte, appName string, aspects ...string) []byte {
	var name strings.Builder
	name.Grow(len(appName) + 16)
	name.WriteString(appName)
	for _, a := range aspects {
		name.WriteByte('.')
		name.WriteString(a)
	}
	sum := sha256.Sum256([]byte(name.String()))
	combined := make([]byte, AppHashPrefixLen+len(identityHash))
	copy(combined, sum[:AppHashPrefixLen])
	copy(combined[AppHashPrefixLen:], identityHash)
	final := sha256.Sum256(combined)
	return final[:DestHashLen:DestHashLen]
}

func TelephonyHash(identityHash []byte) []byte {
	return DestHash(identityHash, AppName, AspectName)
}

var packPool = sync.Pool{
	New: func() any {
		return &encoderBuf{b: make([]byte, 0, packBufCap)}
	},
}

func marshalCompact(v any) ([]byte, error) {
	eb := packPool.Get().(*encoderBuf)
	eb.b = eb.b[:0]
	enc := msgpack.GetEncoder()
	enc.Reset(eb)
	enc.UseCompactInts(true)
	err := enc.Encode(v)
	msgpack.PutEncoder(enc)
	if err != nil {
		eb.b = eb.b[:0]
		packPool.Put(eb)
		return nil, err
	}
	out := make([]byte, len(eb.b))
	copy(out, eb.b)
	if cap(eb.b) > MaxUnpackBytes {
		eb.b = make([]byte, 0, packBufCap)
	} else {
		eb.b = eb.b[:0]
	}
	packPool.Put(eb)
	return out, nil
}

type encoderBuf struct {
	b []byte
}

func (e *encoderBuf) Write(p []byte) (int, error) {
	e.b = append(e.b, p...)
	return len(p), nil
}

func (e *encoderBuf) WriteByte(c byte) error {
	e.b = append(e.b, c)
	return nil
}

func decodeSignals(dec *msgpack.Decoder) ([]int, error) {
	code, err := dec.PeekCode()
	if err != nil {
		return nil, err
	}
	if msgpcode.IsFixedArray(code) || code == msgpcode.Array16 || code == msgpcode.Array32 {
		n, err := dec.DecodeArrayLen()
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, ErrMissingFields
		}
		if n > MaxSignals {
			return nil, ErrPacketTooLarge
		}
		out := make([]int, 0, n)
		for range n {
			num, ok, err := decodeIntOrSkip(dec)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, num)
			}
		}
		return out, nil
	}
	num, ok, err := decodeIntOrSkip(dec)
	if err != nil {
		return nil, err
	}
	if ok {
		return []int{num}, nil
	}
	return nil, ErrMissingFields
}

func decodeFrames(dec *msgpack.Decoder) ([][]byte, error) {
	code, err := dec.PeekCode()
	if err != nil {
		return nil, err
	}
	if msgpcode.IsFixedArray(code) || code == msgpcode.Array16 || code == msgpcode.Array32 {
		n, err := dec.DecodeArrayLen()
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, ErrMissingFields
		}
		if n > MaxFrames {
			return nil, ErrPacketTooLarge
		}
		out := make([][]byte, 0, n)
		for range n {
			b, err := decodeOneFrame(dec)
			if err != nil {
				return nil, err
			}
			if len(b) > 0 {
				out = append(out, b)
			}
		}
		return out, nil
	}
	b, err := decodeOneFrame(dec)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, nil
	}
	return [][]byte{b}, nil
}

func decodeOneFrame(dec *msgpack.Decoder) ([]byte, error) {
	b, err := dec.DecodeBytes()
	if err != nil {
		return nil, err
	}
	if len(b) > MaxFrameBytes {
		return nil, ErrPacketTooLarge
	}
	return b, nil
}

func decodeIntOrSkip(dec *msgpack.Decoder) (int, bool, error) {
	code, err := dec.PeekCode()
	if err != nil {
		return 0, false, err
	}
	if msgpcode.IsFixedNum(code) ||
		code == msgpcode.Int8 || code == msgpcode.Int16 || code == msgpcode.Int32 || code == msgpcode.Int64 ||
		code == msgpcode.Uint8 || code == msgpcode.Uint16 || code == msgpcode.Uint32 || code == msgpcode.Uint64 {
		n, err := dec.DecodeInt()
		return n, true, err
	}
	if err := dec.Skip(); err != nil {
		return 0, false, err
	}
	return 0, false, nil
}
