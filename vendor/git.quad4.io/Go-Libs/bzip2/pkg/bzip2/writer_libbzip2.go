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

static void bz_stream_reset(bz_stream *s) {
	if (s) {
		memset(s, 0, sizeof(bz_stream));
	}
}
*/
import "C"

import (
	"fmt"
	"io"
	"unsafe"
)

const (
	cOutSize = 256 * 1024
	cInSize  = 256 * 1024
)

type Writer struct {
	strmPtr  *C.bz_stream
	dst      io.Writer
	level    int
	inHeap   unsafe.Pointer
	outHeap  unsafe.Pointer
	closed   bool
	abortErr error
}

func cHeapBytes(p unsafe.Pointer, n int) []byte {
	return unsafe.Slice((*byte)(p), n)
}

func (w *Writer) freeBuffers() {
	if w.inHeap != nil {
		C.free(w.inHeap)
		w.inHeap = nil
	}
	if w.outHeap != nil {
		C.free(w.outHeap)
		w.outHeap = nil
	}
}

func (w *Writer) destroyStream() {
	if w.strmPtr != nil {
		C.BZ2_bzCompressEnd(w.strmPtr)
		C.free(unsafe.Pointer(w.strmPtr))
		w.strmPtr = nil
	}
}

func (w *Writer) initStream() error {
	if w.strmPtr == nil {
		w.strmPtr = C.bz_stream_new()
		if w.strmPtr == nil {
			return fmt.Errorf("bzip2: malloc bz_stream")
		}
	} else {
		C.bz_stream_reset(w.strmPtr)
	}
	ret := C.BZ2_bzCompressInit(w.strmPtr, C.int(w.level), 0, 0)
	if ret != 0 {
		if w.strmPtr != nil {
			C.free(unsafe.Pointer(w.strmPtr))
			w.strmPtr = nil
		}
		return fmt.Errorf("bzip2: BZ2_bzCompressInit failed: %d", int(ret))
	}
	return nil
}

func NewWriter(w io.Writer, level int) (*Writer, error) {
	if w == nil {
		return nil, ErrNilWriter
	}
	if level < 1 || level > 9 {
		return nil, ErrLevelRange
	}
	zw := &Writer{dst: w, level: level}
	zw.inHeap = C.malloc(C.size_t(cInSize))
	if zw.inHeap == nil {
		return nil, fmt.Errorf("bzip2: malloc input buffer")
	}
	zw.outHeap = C.malloc(C.size_t(cOutSize))
	if zw.outHeap == nil {
		C.free(zw.inHeap)
		zw.inHeap = nil
		return nil, fmt.Errorf("bzip2: malloc output buffer")
	}
	if err := zw.initStream(); err != nil {
		zw.freeBuffers()
		return nil, err
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
	if w.strmPtr == nil || w.inHeap == nil || w.outHeap == nil {
		err := ErrWriterUninitialized
		w.abortErr = err
		return 0, err
	}
	strm := w.strmPtr
	out := (*C.char)(w.outHeap)
	inOff := 0
	for inOff < len(p) {
		chunkLen := len(p) - inOff
		if chunkLen > cInSize {
			chunkLen = cInSize
		}
		C.memcpy(w.inHeap, unsafe.Pointer(&p[inOff]), C.size_t(chunkLen))
		strm.next_in = (*C.char)(w.inHeap)
		strm.avail_in = C.uint(chunkLen)
		for strm.avail_in > 0 {
			strm.next_out = out
			strm.avail_out = C.uint(cOutSize)
			r := C.BZ2_bzCompress(strm, 0)
			produced := cOutSize - int(strm.avail_out)
			if produced > 0 {
				if err := writeFull(w.dst, cHeapBytes(w.outHeap, produced)); err != nil {
					w.abortErr = err
					return inOff + (chunkLen - int(strm.avail_in)), err
				}
			}
			if r == -8 || r == 1 {
				continue
			}
			if r < 0 {
				err := fmt.Errorf("bzip2: BZ2_bzCompress: %d", int(r))
				w.abortErr = err
				return inOff + (chunkLen - int(strm.avail_in)), err
			}
		}
		inOff += chunkLen
	}
	return len(p), nil
}

func (w *Writer) Close() error {
	if w.closed {
		return ErrClosed
	}
	if w.abortErr != nil {
		w.destroyStream()
		w.freeBuffers()
		return w.abortErr
	}
	if w.strmPtr == nil || w.outHeap == nil {
		err := ErrWriterUninitialized
		w.abortErr = err
		return err
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
			if err := writeFull(w.dst, cHeapBytes(w.outHeap, produced)); err != nil {
				w.abortErr = err
				w.destroyStream()
				w.freeBuffers()
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
			w.destroyStream()
			w.freeBuffers()
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
	w.freeBuffers()
	w.closed = true
	if endRet != 0 {
		return fmt.Errorf("bzip2: BZ2_bzCompressEnd: %d", int(endRet))
	}
	return nil
}

func (w *Writer) Reset(dst io.Writer) error {
	if dst == nil {
		return ErrNilWriter
	}
	if w.level < 1 || w.level > 9 {
		return ErrWriterUninitialized
	}
	w.dst = dst
	w.closed = false
	w.abortErr = nil
	if w.inHeap == nil {
		w.inHeap = C.malloc(C.size_t(cInSize))
		if w.inHeap == nil {
			return fmt.Errorf("bzip2: malloc input buffer")
		}
	}
	if w.outHeap == nil {
		w.outHeap = C.malloc(C.size_t(cOutSize))
		if w.outHeap == nil {
			C.free(w.inHeap)
			w.inHeap = nil
			return fmt.Errorf("bzip2: malloc output buffer")
		}
	}
	if w.strmPtr != nil {
		C.BZ2_bzCompressEnd(w.strmPtr)
	}
	if err := w.initStream(); err != nil {
		w.freeBuffers()
		return err
	}
	return nil
}
