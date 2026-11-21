//go:build cgo

package capi

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <limits.h>

static inline int lxst_size_as_cint(size_t n) {
	if (n > (size_t)INT_MAX) {
		return -1;
	}
	return (int)n;
}
*/
import "C"

import (
	"math"
	"sync"
	"unsafe"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/liblxst"
)

const maxCGoBytes = math.MaxInt32

var versionCString *C.char

func init() {
	versionCString = C.CString(liblxst.Version())
}

//export lxst_version
func lxst_version() *C.char { return versionCString }

//export lxst_last_error
func lxst_last_error(buf *C.char, bufLen C.size_t, written *C.size_t) C.int {
	msg := liblxst.LastError()
	if written != nil {
		*written = 0
	}
	if buf == nil || bufLen == 0 {
		if written != nil {
			*written = sizeFromInt(len(msg))
		}
		if len(msg) > 0 {
			return cCode(liblxst.ErrTruncated)
		}
		return cCode(liblxst.OK)
	}
	n := copyCString(buf, bufLen, msg)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(msg) {
		return cCode(liblxst.ErrTruncated)
	}
	return cCode(liblxst.OK)
}

//export lxst_pack_signalling
func lxst_pack_signalling(signals *C.int, signalCount C.size_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	if signalCount == 0 || signals == nil {
		_, code := liblxst.PackSignalling(nil)
		return cCode(code)
	}
	count, ok := sizeToInt(signalCount)
	if !ok {
		return cCode(liblxst.ErrInvalidArg)
	}
	hdr := unsafe.Slice((*C.int)(unsafe.Pointer(signals)), count)
	sigs := make([]int, len(hdr))
	for i, v := range hdr {
		sigs[i] = int(v)
	}
	data, code := liblxst.PackSignalling(sigs)
	if code != liblxst.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxst_pack_frame
func lxst_pack_frame(codec C.uint8_t, payload *C.uint8_t, payloadLen C.size_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	raw, code := goBytesFromC(payload, payloadLen)
	if code != liblxst.OK {
		return cCode(code)
	}
	data, code := liblxst.PackFrame(byte(codec), raw)
	if code != liblxst.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxst_unpack
func lxst_unpack(data *C.uint8_t, dataLen C.size_t) C.uint64_t {
	raw, code := goBytesFromC(data, dataLen)
	if code != liblxst.OK {
		return 0
	}
	handle, code := liblxst.Unpack(raw)
	if code != liblxst.OK {
		return 0
	}
	return C.uint64_t(handle)
}

//export lxst_packet_destroy
func lxst_packet_destroy(packet C.uint64_t) C.int {
	return cCode(liblxst.PacketDestroy(uint64(packet)))
}

//export lxst_packet_signal_count
func lxst_packet_signal_count(packet C.uint64_t, count *C.size_t) C.int {
	n, code := liblxst.PacketSignalCount(uint64(packet))
	if code != liblxst.OK {
		return cCode(code)
	}
	if count != nil {
		*count = sizeFromInt(n)
	}
	return cCode(liblxst.OK)
}

//export lxst_packet_signal_at
func lxst_packet_signal_at(packet C.uint64_t, index C.size_t, signal *C.int) C.int {
	idx, ok := sizeToInt(index)
	if !ok {
		return cCode(liblxst.ErrInvalidArg)
	}
	v, code := liblxst.PacketSignalAt(uint64(packet), idx)
	if code != liblxst.OK {
		return cCode(code)
	}
	if signal != nil {
		*signal = cInt(v)
	}
	return cCode(liblxst.OK)
}

//export lxst_packet_frame_count
func lxst_packet_frame_count(packet C.uint64_t, count *C.size_t) C.int {
	n, code := liblxst.PacketFrameCount(uint64(packet))
	if code != liblxst.OK {
		return cCode(code)
	}
	if count != nil {
		*count = sizeFromInt(n)
	}
	return cCode(liblxst.OK)
}

//export lxst_packet_frame_at
func lxst_packet_frame_at(packet C.uint64_t, index C.size_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	idx, ok := sizeToInt(index)
	if !ok {
		return cCode(liblxst.ErrInvalidArg)
	}
	data, code := liblxst.PacketFrameAt(uint64(packet), idx)
	if code != liblxst.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxst_split_frame
func lxst_split_frame(frame *C.uint8_t, frameLen C.size_t, codecOut *C.uint8_t,
	payloadOut *C.uint8_t, payloadOutLen C.size_t, payloadWritten *C.size_t) C.int {
	raw, code := goBytesFromC(frame, frameLen)
	if code != liblxst.OK {
		return cCode(code)
	}
	codec, payload, code := liblxst.SplitFrame(raw)
	if code != liblxst.OK {
		return cCode(code)
	}
	if codecOut != nil {
		*codecOut = C.uint8_t(codec)
	}
	return writeBytes(payloadOut, payloadOutLen, payloadWritten, payload)
}

//export lxst_telephony_hash
func lxst_telephony_hash(identityHash *C.uint8_t, identityLen C.size_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	raw, code := goBytesFromC(identityHash, identityLen)
	if code != liblxst.OK {
		return cCode(code)
	}
	data, code := liblxst.TelephonyHash(raw)
	if code != liblxst.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxst_dest_hash
func lxst_dest_hash(identityHash *C.uint8_t, identityLen C.size_t, appName, aspect *C.char,
	out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	raw, code := goBytesFromC(identityHash, identityLen)
	if code != liblxst.OK {
		return cCode(code)
	}
	data, code := liblxst.DestHash(raw, C.GoString(appName), C.GoString(aspect))
	if code != liblxst.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxst_signal_preferred_mode
func lxst_signal_preferred_mode(mode C.int) C.int {
	return cCode(liblxst.SignalPreferredMode(int(mode)))
}

//export lxst_signal_preferred_profile
func lxst_signal_preferred_profile(profile C.int) C.int {
	return cCode(liblxst.SignalPreferredProfile(int(profile)))
}

//export lxst_profile_from_name
func lxst_profile_from_name(name *C.char) C.int {
	return cCode(liblxst.ProfileFromName(C.GoString(name)))
}

func goBytesFromC(ptr *C.uint8_t, n C.size_t) ([]byte, int) {
	if n == 0 {
		return nil, liblxst.OK
	}
	if ptr == nil {
		return nil, liblxst.ErrInvalidArg
	}
	cint := C.lxst_size_as_cint(n)
	if cint < 0 {
		return nil, liblxst.ErrInvalidArg
	}
	return C.GoBytes(unsafe.Pointer(ptr), cint), liblxst.OK
}

func writeBytes(out *C.uint8_t, outLen C.size_t, written *C.size_t, src []byte) C.int {
	if written != nil {
		*written = 0
	}
	if out == nil || outLen == 0 {
		if written != nil {
			*written = sizeFromInt(len(src))
		}
		if len(src) > 0 {
			return cCode(liblxst.ErrTruncated)
		}
		return cCode(liblxst.OK)
	}
	n := copyCBytes(out, outLen, src)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(src) {
		return cCode(liblxst.ErrTruncated)
	}
	return cCode(liblxst.OK)
}

func copyCString(dst *C.char, capacity C.size_t, s string) int {
	limit, ok := sizeToInt(capacity)
	if !ok || limit == 0 {
		return 0
	}
	room := limit - 1
	n := len(s)
	if n > room {
		n = room
	}
	if n > 0 {
		C.memcpy(unsafe.Pointer(dst), unsafe.Pointer(unsafe.StringData(s)), sizeFromInt(n))
	}
	p := unsafe.Slice((*byte)(unsafe.Pointer(dst)), limit)
	p[n] = 0
	return n
}

func copyCBytes(dst *C.uint8_t, capacity C.size_t, src []byte) int {
	limit, ok := sizeToInt(capacity)
	if !ok || limit == 0 {
		return 0
	}
	n := len(src)
	if n > limit {
		n = limit
	}
	if n > 0 {
		C.memcpy(unsafe.Pointer(dst), unsafe.Pointer(&src[0]), sizeFromInt(n))
	}
	return n
}

func sizeToInt(n C.size_t) (int, bool) {
	if n > C.size_t(maxCGoBytes) {
		return 0, false
	}
	return int(n), true
}

func sizeFromInt(n int) C.size_t {
	if n < 0 {
		return 0
	}
	return C.size_t(n)
}

func cCode(code int) C.int {
	switch code {
	case liblxst.OK:
		return 0
	case liblxst.ErrInvalidArg:
		return 1
	case liblxst.ErrInvalidHandle:
		return 2
	case liblxst.ErrInternal:
		return 6
	case liblxst.ErrTruncated:
		return 8
	default:
		return 6
	}
}

// #nosec G115 -- LXST signal values fit the C ABI int type
func cInt(v int) C.int {
	return C.int(v)
}

var _ = sync.Mutex{}
