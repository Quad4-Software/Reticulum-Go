// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
)

func TestFormattedErrorsWrapSentinels(t *testing.T) {
	hash := []byte{0x01, 0x02}
	if !errors.Is(ErrLinkNoPathf(hash), ErrLinkNoPath) {
		t.Fatal("ErrLinkNoPathf should wrap ErrLinkNoPath")
	}
	if !errors.Is(ErrNoPathToDestinationf(hash), ErrNoPathToDestination) {
		t.Fatal("ErrNoPathToDestinationf should wrap ErrNoPathToDestination")
	}
	if !errors.Is(ErrIdentityNotFoundf(hash), ErrIdentityNotFound) {
		t.Fatal("ErrIdentityNotFoundf should wrap ErrIdentityNotFound")
	}
	if !errors.Is(ErrConfigf("bad port %d", 0), ErrConfig) {
		t.Fatal("ErrConfigf should wrap ErrConfig")
	}
	if !errors.Is(ErrCorruptionf("table"), ErrCorruption) {
		t.Fatal("ErrCorruptionf should wrap ErrCorruption")
	}
	if !errors.Is(ErrSandboxf("pledge"), ErrSandbox) {
		t.Fatal("ErrSandboxf should wrap ErrSandbox")
	}
	if !errors.Is(ErrPortConflictf("udp :4242"), ErrPortConflict) {
		t.Fatal("ErrPortConflictf should wrap ErrPortConflict")
	}
	if !errors.Is(ErrDiskf("persist"), ErrDisk) {
		t.Fatal("ErrDiskf should wrap ErrDisk")
	}
	if !errors.Is(ErrOOMf("budget"), ErrOOM) {
		t.Fatal("ErrOOMf should wrap ErrOOM")
	}
	if !errors.Is(ErrCPUf("throttle"), ErrCPU) {
		t.Fatal("ErrCPUf should wrap ErrCPU")
	}
}

func TestClassifyIOError(t *testing.T) {
	if !errors.Is(ClassifyIOError(syscall.EADDRINUSE), ErrPortConflict) {
		t.Fatal("EADDRINUSE should classify as ErrPortConflict")
	}
	if !errors.Is(ClassifyIOError(syscall.ENOSPC), ErrDisk) {
		t.Fatal("ENOSPC should classify as ErrDisk")
	}
	if !errors.Is(ClassifyIOError(syscall.ENOMEM), ErrOOM) {
		t.Fatal("ENOMEM should classify as ErrOOM")
	}
	plain := errors.New("something else")
	if ClassifyIOError(plain) != plain {
		t.Fatal("unknown errors should pass through")
	}
	if ClassifyIOError(nil) != nil {
		t.Fatal("nil should stay nil")
	}

	already := fmt.Errorf("%w: detail", ErrDisk)
	if ClassifyIOError(already) != already {
		t.Fatal("already-classified errors should pass through unchanged")
	}
}

func TestIsPortConflictUnwrapsNetAndSyscall(t *testing.T) {
	if IsPortConflict(nil) {
		t.Fatal("nil is not a port conflict")
	}
	if !IsPortConflict(ErrPortConflict) {
		t.Fatal("sentinel should match")
	}

	sys := &os.SyscallError{Syscall: "bind", Err: syscall.EADDRINUSE}
	op := &net.OpError{Op: "listen", Net: "tcp", Err: sys}
	if !IsPortConflict(op) {
		t.Fatal("net.OpError wrapping SyscallError EADDRINUSE should match")
	}
	if !IsPortConflict(errors.New("listen tcp :80: bind: address already in use")) {
		t.Fatal("message fallback should match")
	}
	if IsPortConflict(errors.New("connection refused")) {
		t.Fatal("unrelated error should not match")
	}
}

func TestIsDiskFullAndIsOOM(t *testing.T) {
	if IsDiskFull(nil) || IsOOM(nil) {
		t.Fatal("nil should be false")
	}
	if !IsDiskFull(ErrDisk) {
		t.Fatal("ErrDisk should match")
	}
	if !IsDiskFull(errors.New("write: no space left on device")) {
		t.Fatal("message fallback should match disk full")
	}
	if !IsOOM(ErrOOM) {
		t.Fatal("ErrOOM should match")
	}
	if !IsOOM(ErrMemoryBudgetExceeded) {
		t.Fatal("memory budget should count as OOM")
	}
	if !IsOOM(fmt.Errorf("alloc: %w", syscall.ENOMEM)) {
		t.Fatal("ENOMEM wrap should match")
	}
	if !IsOOM(errors.New("cannot allocate memory")) {
		t.Fatal("message fallback should match OOM")
	}
}

func TestWrapListenErrorRealPortConflict(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	_, err = net.Listen("tcp", ln.Addr().String())
	if err == nil {
		t.Fatal("expected second listen to fail")
	}
	wrapped := WrapListenError(err)
	if !errors.Is(wrapped, ErrPortConflict) {
		t.Fatalf("WrapListenError should classify as ErrPortConflict, got %v", wrapped)
	}
	if !errors.Is(wrapped, err) {
		t.Fatalf("original listen error should remain unwrapable, got %v", wrapped)
	}
	if WrapListenError(nil) != nil {
		t.Fatal("nil should stay nil")
	}
}

func TestWrapWriteError(t *testing.T) {
	if WrapWriteError(nil) != nil {
		t.Fatal("nil should stay nil")
	}
	if WrapWriteError(io.ErrShortWrite) != io.ErrShortWrite {
		t.Fatal("short write should pass through unchanged")
	}
	if !errors.Is(WrapWriteError(syscall.ENOSPC), ErrDisk) {
		t.Fatal("ENOSPC should classify as ErrDisk")
	}
	if !errors.Is(WrapWriteError(syscall.ENOMEM), ErrOOM) {
		t.Fatal("ENOMEM should classify as ErrOOM")
	}
	plain := errors.New("permission denied")
	if WrapWriteError(plain) != plain {
		t.Fatal("unknown write errors should pass through")
	}
}

func TestConfigValidateUsesErrConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SharedInstancePort = 0
	err := cfg.Validate()
	if !errors.Is(err, ErrConfig) {
		t.Fatalf("Validate should wrap ErrConfig, got %v", err)
	}
}
