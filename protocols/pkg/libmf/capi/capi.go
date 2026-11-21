//go:build cgo

package capi

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <limits.h>

static inline int mf_size_as_cint(size_t n) {
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

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/libmf"
)

const maxCGoBytes = math.MaxInt32

var versionCString *C.char

func init() {
	versionCString = C.CString(libmf.Version())
}

//export mf_version
func mf_version() *C.char { return versionCString }

//export mf_last_error
func mf_last_error(buf *C.char, bufLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	msg := libmf.LastError()
	if buf == nil || bufLen == 0 {
		if written != nil {
			*written = sizeFromInt(len(msg))
		}
		if len(msg) > 0 {
			return cCode(libmf.ErrTruncated)
		}
		return cCode(libmf.OK)
	}
	n := copyCString(buf, bufLen, msg)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(msg) {
		return cCode(libmf.ErrTruncated)
	}
	return cCode(libmf.OK)
}

//export mf_pack
func mf_pack(sender *C.uint8_t, senderLen C.size_t, text *C.char, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	s, code := goBytesFromC(sender, senderLen)
	if code != libmf.OK {
		return cCode(code)
	}
	data, code := libmf.Pack(s, C.GoString(text))
	if code != libmf.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export mf_unpack
func mf_unpack(data *C.uint8_t, dataLen C.size_t,
	senderOut *C.uint8_t, senderOutLen C.size_t, senderWritten *C.size_t,
	textOut *C.char, textOutLen C.size_t, textWritten *C.size_t) C.int {
	raw, code := goBytesFromC(data, dataLen)
	if code != libmf.OK {
		return cCode(code)
	}
	sender, text, code := libmf.Unpack(raw)
	if code != libmf.OK {
		return cCode(code)
	}
	if c := writeBytes(senderOut, senderOutLen, senderWritten, sender); c != cCode(libmf.OK) {
		return c
	}
	return writeString(textOut, textOutLen, textWritten, text)
}

func goBytesFromC(ptr *C.uint8_t, n C.size_t) ([]byte, int) {
	if n == 0 {
		return nil, libmf.OK
	}
	if ptr == nil {
		return nil, libmf.ErrInvalidArg
	}
	cint := C.mf_size_as_cint(n)
	if cint < 0 {
		return nil, libmf.ErrInvalidArg
	}
	return C.GoBytes(unsafe.Pointer(ptr), cint), libmf.OK
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
			return cCode(libmf.ErrTruncated)
		}
		return cCode(libmf.OK)
	}
	n := copyCBytes(out, outLen, src)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(src) {
		return cCode(libmf.ErrTruncated)
	}
	return cCode(libmf.OK)
}

func writeString(buf *C.char, bufLen C.size_t, written *C.size_t, s string) C.int {
	if written != nil {
		*written = 0
	}
	if buf == nil || bufLen == 0 {
		if written != nil {
			*written = sizeFromInt(len(s))
		}
		if len(s) > 0 {
			return cCode(libmf.ErrTruncated)
		}
		return cCode(libmf.OK)
	}
	n := copyCString(buf, bufLen, s)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(s) {
		return cCode(libmf.ErrTruncated)
	}
	return cCode(libmf.OK)
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
	case libmf.OK:
		return 0
	case libmf.ErrInvalidArg:
		return 1
	case libmf.ErrInternal:
		return 6
	case libmf.ErrTruncated:
		return 8
	default:
		return 6
	}
}

var _ = sync.Mutex{}
