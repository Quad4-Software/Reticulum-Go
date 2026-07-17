// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"errors"
	"io"
	"syscall"
	"testing"
)

// FuzzClassifyIOError ensures ClassifyIOError never panics and only maps
// known OS conditions onto library sentinels.
func FuzzClassifyIOError(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("address already in use"))
	f.Add([]byte("no space left on device"))
	f.Add([]byte("cannot allocate memory"))
	f.Add([]byte("permission denied"))
	f.Add([]byte{0x00, 0xff})

	f.Fuzz(func(t *testing.T, msg []byte) {
		if len(msg) > 1<<12 {
			t.Skip()
		}
		err := errors.New(string(msg))
		got := ClassifyIOError(err)
		if got == nil {
			t.Fatal("ClassifyIOError must not return nil for non-nil input")
		}
		switch {
		case errors.Is(got, ErrPortConflict), errors.Is(got, ErrDisk), errors.Is(got, ErrOOM):
			return
		case got == err:
			return
		default:
			t.Fatalf("unexpected classification result: %v", got)
		}
	})
}

func TestWrapWriteErrorShortWriteSeed(t *testing.T) {
	if WrapWriteError(io.ErrShortWrite) != io.ErrShortWrite {
		t.Fatal("short write must pass through")
	}
	if !errors.Is(WrapWriteError(syscall.ENOSPC), ErrDisk) {
		t.Fatal("ENOSPC must classify as ErrDisk")
	}
}
