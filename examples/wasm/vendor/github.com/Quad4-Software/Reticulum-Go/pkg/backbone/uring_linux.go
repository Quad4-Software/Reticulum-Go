// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package backbone

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const uringProbeEntries = 64

type uringPoller struct {
	*epollPoller
}

func newUringPoller() (poller, error) {
	if !UringProbeAllowed() {
		return nil, fmt.Errorf("io_uring unavailable")
	}
	if err := probeIOUring(); err != nil {
		return nil, err
	}
	ep, err := newEpollPoller()
	if err != nil {
		return nil, err
	}
	return &uringPoller{epollPoller: ep.(*epollPoller)}, nil
}

func probeIOUring() error {
	// struct io_uring_params is 120 bytes on every kernel since 5.1 and the
	// kernel copies the full struct out unconditionally. Allocate 128 bytes
	// so the copyout can never overwrite the stack frame.
	var params [128]byte
	// #nosec G103 -- io_uring_setup requires passing a struct pointer to the syscall
	fd, _, errno := syscall.Syscall(unix.SYS_IO_URING_SETUP, uintptr(uringProbeEntries), uintptr(unsafe.Pointer(&params[0])), 0)
	if errno != 0 {
		return fmt.Errorf("io_uring_setup: %w", errno)
	}
	// #nosec G115 -- io_uring probe fd is a small kernel fd
	_ = unix.Close(socketFD(fd))
	return nil
}

// UringProbeAllowed reports whether this process should attempt io_uring_setup.
// GitHub Actions and similar CI sandboxes often deny the syscall with SIGSYS.
func UringProbeAllowed() bool {
	if os.Getenv("CI") != "" && os.Getenv("RETICULUM_ENABLE_IO_URING") == "" {
		return false
	}
	return true
}
