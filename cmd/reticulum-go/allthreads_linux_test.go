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

// TestAllThreadsSyscallUsable uses the daemon import graph. purego/fakecgo
// (OpenCL via -tags lxstamp_gpu) or real cgo sets runtime.iscgo and makes
// AllThreadsSyscall panic, which crashes Landlock on kernels before ABI 8.
func TestAllThreadsSyscallUsable(t *testing.T) {
	if os.Getenv("RETICULUM_QEMU_USER") == "1" {
		t.Skip("AllThreadsSyscall is unreliable under qemu-user")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("AllThreadsSyscall panicked: %v\n"+
				"Default daemon builds must not import purego/fakecgo or enable cgo.\n"+
				"OpenCL stamping is opt-in with -tags lxstamp_gpu.", r)
		}
	}()

	_, _, errno := syscall.AllThreadsSyscall(unix.SYS_GETPID, 0, 0, 0)
	if errno != 0 {
		t.Fatalf("AllThreadsSyscall(GETPID): %v", errno)
	}
}
