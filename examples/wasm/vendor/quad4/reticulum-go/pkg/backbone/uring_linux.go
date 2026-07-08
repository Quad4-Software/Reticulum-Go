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
	if !uringProbeAllowed() {
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
	params := struct {
		sqEntries    uint32
		cqEntries    uint32
		flags        uint32
		sqThread     uint32
		sqCpu        uint32
		pad          uint32
		featureFlags uint32
		wqFd         int32
		resv         [3]uint32
		sqOff        [5]uint32
		cqOff        [5]uint32
	}{
		sqEntries: uringProbeEntries,
		cqEntries: uringProbeEntries * 2,
	}
	// #nosec G103 -- io_uring_setup requires passing a struct pointer to the syscall
	fd, _, errno := syscall.Syscall(unix.SYS_IO_URING_SETUP, uintptr(uringProbeEntries), uintptr(unsafe.Pointer(&params)), 0)
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
	return uringProbeAllowed()
}

func uringProbeAllowed() bool {
	if os.Getenv("CI") != "" && os.Getenv("RETICULUM_ENABLE_IO_URING") == "" {
		return false
	}
	return true
}
