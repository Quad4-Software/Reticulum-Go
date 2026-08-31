// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package sandbox

import (
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// TestAllThreadsSyscallUsable fails on the fakecgo panic that aborts Landlock
// under CGO_ENABLED=0. Real cgo returns ENOTSUP here and is skipped (release
// builds set CGO_ENABLED=0). Daemon coverage lives in cmd/reticulum-go.
func TestAllThreadsSyscallUsable(t *testing.T) {
	if os.Getenv("RETICULUM_QEMU_USER") == "1" {
		t.Skip("AllThreadsSyscall is unreliable under qemu-user")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("AllThreadsSyscall panicked: %v\n"+
				"Sandbox package must stay free of purego/fakecgo.", r)
		}
	}()

	_, _, errno := syscall.AllThreadsSyscall(unix.SYS_GETPID, 0, 0, 0)
	if errno == syscall.ENOTSUP {
		t.Skip("AllThreadsSyscall ENOTSUP (real cgo linked; release builds use CGO_ENABLED=0)")
	}
	if errno != 0 {
		t.Fatalf("AllThreadsSyscall(GETPID): %v", errno)
	}
}
