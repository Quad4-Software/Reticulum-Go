// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package securemem

import (
	"golang.org/x/sys/unix"
)

// mapSecret reserves dataSize+2*pageSize anonymous memory. The outer pages
// stay PROT_NONE as guard pages; the inner region is read-write. Returns the
// whole mapping (for unmapSecret) and the usable inner region.
func mapSecret(dataSize int) (mapping, usable []byte, err error) {
	page := unix.Getpagesize()
	dataSize = roundUp(dataSize, page)
	total := dataSize + 2*page
	m, err := unix.Mmap(-1, 0, total, unix.PROT_NONE, unix.MAP_PRIVATE|unix.MAP_ANON)
	if err != nil {
		return nil, nil, err
	}
	usable = m[page : page+dataSize]
	if err := unix.Mprotect(usable, unix.PROT_READ|unix.PROT_WRITE); err != nil {
		_ = unix.Munmap(m)
		return nil, nil, err
	}
	// MADV_DONTDUMP keeps the secret out of core dumps and MADV_WIPEONFORK
	// zeroes the child's copy on fork. Best effort: an older kernel denying
	// madvise still leaves a working, locked region.
	_ = unix.Madvise(usable, unix.MADV_DONTDUMP)
	_ = unix.Madvise(usable, unix.MADV_WIPEONFORK)
	return m, usable, nil
}

func unmapSecret(mapping []byte) {
	_ = unix.Munmap(mapping)
}

// protectRange toggles a region between PROT_NONE and read-write for the
// idle-protection mode.
func protectRange(b []byte, rw bool) error {
	prot := unix.PROT_NONE
	if rw {
		prot = unix.PROT_READ | unix.PROT_WRITE
	}
	return unix.Mprotect(b, prot)
}

func lockRange(b []byte) error { return unix.Mlock(b) }

func unlockRange(b []byte) { _ = unix.Munlock(b) }

const isHeapFallback = false
