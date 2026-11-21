// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build windows

package securemem

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	winPageNoAccess  = 0x01
	winPageReadWrite = 0x04
	winMemCommit     = 0x1000
	winMemReserve    = 0x2000
	winMemRelease    = 0x8000
)

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procVirtualAlloc        = kernel32.NewProc("VirtualAlloc")
	procVirtualFree         = kernel32.NewProc("VirtualFree")
	procVirtualProtect      = kernel32.NewProc("VirtualProtect")
	procVirtualLock         = kernel32.NewProc("VirtualLock")
	procVirtualUnlock       = kernel32.NewProc("VirtualUnlock")
	procGetSystemInfo       = kernel32.NewProc("GetSystemInfo")
	winPageSize             = initWinPageSize()
	winVirtualProtectFailed bool
)

func initWinPageSize() int {
	var si [48]byte
	procGetSystemInfo.Call(uintptr(unsafe.Pointer(&si[0])))
	return int(*(*uint32)(unsafe.Pointer(&si[8])))
}

func winVirtualAlloc(size int, protect uint32) (uintptr, error) {
	addr, _, err := procVirtualAlloc.Call(0, uintptr(size), winMemCommit|winMemReserve, uintptr(protect))
	if addr == 0 {
		return 0, err
	}
	return addr, nil
}

func winVirtualProtect(addr uintptr, size int, protect uint32) error {
	var old uint32
	r, _, err := procVirtualProtect.Call(addr, uintptr(size), uintptr(protect), uintptr(unsafe.Pointer(&old)))
	if r == 0 {
		return err
	}
	return nil
}

func mapSecret(dataSize int) (mapping, usable []byte, err error) {
	page := winPageSize
	if page <= 0 {
		page = 4096
	}
	dataSize = roundUp(dataSize, page)
	total := dataSize + 2*page
	base, err := winVirtualAlloc(total, winPageNoAccess)
	if err != nil {
		return nil, nil, err
	}
	usableAddr := base + uintptr(page)
	if err := winVirtualProtect(usableAddr, dataSize, winPageReadWrite); err != nil {
		procVirtualFree.Call(base, 0, winMemRelease)
		return nil, nil, err
	}
	mapping = unsafe.Slice((*byte)(unsafe.Pointer(base)), total)
	usable = unsafe.Slice((*byte)(unsafe.Pointer(usableAddr)), dataSize)
	return mapping, usable, nil
}

func unmapSecret(mapping []byte) {
	procVirtualFree.Call(uintptr(unsafe.Pointer(&mapping[0])), 0, winMemRelease)
}

func protectRange(b []byte, rw bool) error {
	prot := uint32(winPageNoAccess)
	if rw {
		prot = winPageReadWrite
	}
	err := winVirtualProtect(uintptr(unsafe.Pointer(&b[0])), len(b), prot)
	if err != nil {
		winVirtualProtectFailed = true
	}
	return err
}

func lockRange(b []byte) error {
	r, _, err := procVirtualLock.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	if r == 0 {
		return err
	}
	return nil
}

func unlockRange(b []byte) {
	procVirtualUnlock.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
}

const isHeapFallback = false
