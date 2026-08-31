// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package main

import (
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// TestAllThreadsSyscallUsable guards the daemon import graph under CGO_ENABLED=0.
// purego/fakecgo (-tags lxstamp_gpu) sets runtime.iscgo without libc setegid
// hooks, so AllThreadsSyscall panics and Landlock aborts on kernels before ABI 8.
// With real cgo (default go test when a C compiler is present), net/os/user link
// cgo and AllThreadsSyscall returns ENOTSUP; go-landlock then uses libpsx.
func TestAllThreadsSyscallUsable(t *testing.T) {
	if os.Getenv("RETICULUM_QEMU_USER") == "1" {
		t.Skip("AllThreadsSyscall is unreliable under qemu-user")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("AllThreadsSyscall panicked: %v\n"+
				"Default daemon builds must not import purego/fakecgo.\n"+
				"OpenCL stamping is opt-in with -tags lxstamp_gpu. Release builds use CGO_ENABLED=0.", r)
		}
	}()

	_, _, errno := syscall.AllThreadsSyscall(unix.SYS_GETPID, 0, 0, 0)
	if errno == syscall.ENOTSUP {
		t.Skip("AllThreadsSyscall ENOTSUP (real cgo linked via net/os/user; release builds use CGO_ENABLED=0)")
	}
	if errno != 0 {
		t.Fatalf("AllThreadsSyscall(GETPID): %v", errno)
	}
}
