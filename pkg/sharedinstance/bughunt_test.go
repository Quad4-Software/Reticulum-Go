// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sharedinstance

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// TestBughuntRecvFramedMaxSizeZeroMustNotAllocate guards the allocation
// bomb where maxSize<=0 skipped the length check and make()'d the wire size.
func TestBughuntRecvFramedMaxSizeZeroMustNotAllocate(t *testing.T) {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 1<<28)
	done := make(chan error, 1)
	go func() {
		_, err := RecvFramed(bytes.NewReader(hdr[:]), 0)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("maxSize=0 accepted a 256MiB claimed frame")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RecvFramed hung or is allocating unboundedly")
	}

	_, err := RecvFramed(bytes.NewReader(hdr[:]), -1)
	if err == nil {
		t.Fatal("maxSize=-1 accepted oversize frame")
	}
}

func TestBughuntRecvFramedExtendedLengthCapped(t *testing.T) {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, int32(-1))
	_ = binary.Write(&buf, binary.BigEndian, uint64(1<<40))
	_, err := RecvFramed(&buf, 0)
	if err == nil {
		t.Fatal("extended 1TiB frame accepted with maxSize=0")
	}
}
