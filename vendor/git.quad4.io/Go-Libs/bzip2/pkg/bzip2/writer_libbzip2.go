// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

//go:build libbzip2

// Package bzip2 Writer with this build tag uses the system libbz2 (C) for compression.
// Build with: go build -tags libbzip2
// Requires libbz2 headers and -lbz2 at link time.

package bzip2

/*
#cgo LDFLAGS: -lbz2
#include <bzlib.h>
#include <stdlib.h>
#include <string.h>

static bz_stream *bz_stream_new(void) {
	bz_stream *s = (bz_stream *)malloc(sizeof(bz_stream));
	if (s) {
		memset(s, 0, sizeof(bz_stream));
	}
	return s;
}
*/
import "C"

import (
	"fmt"
	"io"
	"unsafe"
)

const cOutSize = 256 * 1024

type Writer struct {
	strmPtr  *C.bz_stream
	dst      io.Writer
	level    int
	outHeap  unsafe.Pointer
	closed   bool
	abortErr error
}

func NewWriter(w io.Writer, level int) (*Writer, error) {
	if w == nil {
		return nil, ErrNilWriter
	}
	if level < 1 || level > 9 {
		return nil, ErrLevelRange
	}
	zw := &Writer{dst: w, level: level}
	zw.strmPtr = C.bz_stream_new()
	if zw.strmPtr == nil {
		return nil, fmt.Errorf("bzip2: malloc bz_stream")
	}
	zw.outHeap = C.malloc(C.size_t(cOutSize))
	if zw.outHeap == nil {
		C.free(unsafe.Pointer(zw.strmPtr))
		zw.strmPtr = nil
		return nil, fmt.Errorf("bzip2: malloc output buffer")
	}
	ret := C.BZ2_bzCompressInit(zw.strmPtr, C.int(level), 0, 0)
	if ret != 0 {
		C.free(zw.outHeap)
		zw.outHeap = nil
		C.free(unsafe.Pointer(zw.strmPtr))
		zw.strmPtr = nil
		return nil, fmt.Errorf("bzip2: BZ2_bzCompressInit failed: %d", int(ret))
	}
	return zw, nil
}

func (w *Writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, ErrClosed
	}
	if w.abortErr != nil {
		return 0, w.abortErr
	}
	if len(p) == 0 {
		return 0, nil
	}
	cin := C.CBytes(p)
	defer C.free(unsafe.Pointer(cin))
	strm := w.strmPtr
	strm.next_in = (*C.char)(cin)
	strm.avail_in = C.uint(len(p))
	out := (*C.char)(w.outHeap)
	for strm.avail_in > 0 {
		strm.next_out = out
		strm.avail_out = C.uint(cOutSize)
		r := C.BZ2_bzCompress(strm, 0)
		produced := cOutSize - int(strm.avail_out)
		if produced > 0 {
			chunk := C.GoBytes(unsafe.Pointer(out), C.int(produced))
			if err := writeFull(w.dst, chunk); err != nil {
				w.abortErr = err
				return len(p) - int(strm.avail_in), err
			}
		}
		if r == -8 {
			continue
		}
		if r == 1 {
			continue
		}
		if r < 0 {
			err := fmt.Errorf("bzip2: BZ2_bzCompress: %d", int(r))
			w.abortErr = err
			return len(p) - int(strm.avail_in), err
		}
	}
	return len(p), nil
}

func (w *Writer) Close() error {
	if w.closed {
		return ErrClosed
	}
	if w.abortErr != nil {
		if w.outHeap != nil {
			C.free(w.outHeap)
			w.outHeap = nil
		}
		if w.strmPtr != nil {
			C.BZ2_bzCompressEnd(w.strmPtr)
			C.free(unsafe.Pointer(w.strmPtr))
			w.strmPtr = nil
		}
		return w.abortErr
	}
	strm := w.strmPtr
	strm.next_in = nil
	strm.avail_in = 0
	out := (*C.char)(w.outHeap)
	for {
		strm.next_out = out
		strm.avail_out = C.uint(cOutSize)
		r := C.BZ2_bzCompress(strm, 2)
		produced := cOutSize - int(strm.avail_out)
		if produced > 0 {
			chunk := C.GoBytes(unsafe.Pointer(out), C.int(produced))
			if err := writeFull(w.dst, chunk); err != nil {
				w.abortErr = err
				C.BZ2_bzCompressEnd(strm)
				if w.outHeap != nil {
					C.free(w.outHeap)
					w.outHeap = nil
				}
				if w.strmPtr != nil {
					C.free(unsafe.Pointer(w.strmPtr))
					w.strmPtr = nil
				}
				return err
			}
		}
		if r == 4 {
			break
		}
		if r == 3 || r == 1 {
			continue
		}
		if r == -8 {
			continue
		}
		if r < 0 {
			C.BZ2_bzCompressEnd(strm)
			if w.outHeap != nil {
				C.free(w.outHeap)
				w.outHeap = nil
			}
			if w.strmPtr != nil {
				C.free(unsafe.Pointer(w.strmPtr))
				w.strmPtr = nil
			}
			err := fmt.Errorf("bzip2: BZ2_bzCompress finish: %d", int(r))
			w.abortErr = err
			return err
		}
	}
	endRet := C.BZ2_bzCompressEnd(strm)
	if w.strmPtr != nil {
		C.free(unsafe.Pointer(w.strmPtr))
		w.strmPtr = nil
	}
	if w.outHeap != nil {
		C.free(w.outHeap)
		w.outHeap = nil
	}
	w.closed = true
	if endRet != 0 {
		return fmt.Errorf("bzip2: BZ2_bzCompressEnd: %d", int(endRet))
	}
	return nil
}
