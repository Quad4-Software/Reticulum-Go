// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build openbsd

package securemem

import (
	"golang.org/x/sys/unix"
)

// MAP_CONCEAL keeps the region out of core dumps and fork on OpenBSD.
func mapSecret(dataSize int) (mapping, usable []byte, err error) {
	page := unix.Getpagesize()
	dataSize = roundUp(dataSize, page)
	total := dataSize + 2*page
	m, err := unix.Mmap(-1, 0, total, unix.PROT_NONE, unix.MAP_PRIVATE|unix.MAP_ANON|unix.MAP_CONCEAL)
	if err != nil {
		return nil, nil, err
	}
	usable = m[page : page+dataSize]
	if err := unix.Mprotect(usable, unix.PROT_READ|unix.PROT_WRITE); err != nil {
		_ = unix.Munmap(m)
		return nil, nil, err
	}
	return m, usable, nil
}

func unmapSecret(mapping []byte) {
	_ = unix.Munmap(mapping)
}

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
