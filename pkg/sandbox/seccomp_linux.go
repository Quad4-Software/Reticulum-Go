// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux && !tinygo

package sandbox

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	seccompDataNrOffset   = 0
	seccompDataArchOffset = 4
	seccompRetErrnoEPERM  = unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)
)

func seccompEnabled(cfg *common.ReticulumConfig) bool {
	if cfg == nil {
		return true
	}
	if !cfg.EnableSandbox {
		return false
	}
	return cfg.EnableSeccomp
}

func applySeccomp(cfg *common.ReticulumConfig) error {
	if !seccompEnabled(cfg) {
		debug.Log(debug.DebugInfo, "Seccomp disabled by configuration")
		return nil
	}
	mode, err := installSeccompFilter()
	if err != nil {
		debug.Log(debug.DebugError, "Seccomp filter install failed (continuing)", "error", err)
		warnSoftUnavailable("seccomp", err.Error())
		if cfg != nil && cfg.SandboxStrict {
			return err
		}
		return nil
	}
	debug.Log(debug.DebugInfo, "Seccomp filter applied", "arch", runtime.GOARCH, "mode", mode)
	return nil
}

// installSeccompFilter installs the denylist BPF filter.
// Preference order:
//  1. seccomp(SECCOMP_SET_MODE_FILTER, TSYNC) process-wide
//  2. AllThreadsSyscall seccomp without TSYNC (kernels without the flag)
//  3. AllThreadsSyscall prctl(PR_SET_SECCOMP) when the seccomp syscall is missing
func installSeccompFilter() (string, error) {
	if os.Getenv("RETICULUM_QEMU_USER") == "1" {
		return "", fmt.Errorf("seccomp skipped under qemu-user")
	}

	prog, err := buildSeccompProg()
	if err != nil {
		return "", err
	}

	_, _, errno := unix.Syscall(
		unix.SYS_SECCOMP,
		uintptr(unix.SECCOMP_SET_MODE_FILTER),
		uintptr(unix.SECCOMP_FILTER_FLAG_TSYNC),
		uintptr(unsafe.Pointer(prog)), // #nosec G103 - required for SECCOMP_SET_MODE_FILTER
	)
	if errno == 0 {
		return "tsync", nil
	}

	switch errno {
	case unix.EINVAL, unix.ESRCH, unix.ENOSYS:
		mode, err := installSeccompAllThreads(prog)
		if err != nil {
			return "", fmt.Errorf("seccomp tsync unavailable (%v), fallback failed: %w", errno, err)
		}
		return mode, nil
	default:
		return "", errno
	}
}

func installSeccompAllThreads(prog *unix.SockFprog) (string, error) {
	_, _, errno := syscall.AllThreadsSyscall(
		unix.SYS_SECCOMP,
		uintptr(unix.SECCOMP_SET_MODE_FILTER),
		0,
		uintptr(unsafe.Pointer(prog)), // #nosec G103 - required for SECCOMP_SET_MODE_FILTER
	)
	if errno == 0 {
		return "all_threads", nil
	}
	if errno != unix.ENOSYS {
		return "", errno
	}

	// Kernels without CONFIG_SECCOMP_FILTER's seccomp syscall still accept
	// filter install through prctl on some older builds.
	_, _, errno = syscall.AllThreadsSyscall(
		unix.SYS_PRCTL,
		uintptr(unix.PR_SET_SECCOMP),
		uintptr(unix.SECCOMP_MODE_FILTER),
		uintptr(unsafe.Pointer(prog)), // #nosec G103 - required for PR_SET_SECCOMP filter install
	)
	if errno != 0 {
		return "", fmt.Errorf("prctl seccomp: %w", errno)
	}
	return "prctl", nil
}

func buildSeccompProg() (*unix.SockFprog, error) {
	arch, denied, err := seccompPolicy()
	if err != nil {
		return nil, err
	}

	filter := make([]unix.SockFilter, 0, 8+2*len(denied))
	filter = append(filter,
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, seccompDataArchOffset),
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, arch, 1, 0),
		bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_KILL_THREAD),
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, seccompDataNrOffset),
	)
	for _, nr := range denied {
		filter = append(filter,
			bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, uint32(nr), 0, 1), // #nosec G115 - syscall numbers are small positive constants
			bpfStmt(unix.BPF_RET|unix.BPF_K, seccompRetErrnoEPERM),
		)
	}
	filter = append(filter, bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ALLOW))

	return &unix.SockFprog{
		Len:    uint16(len(filter)), // #nosec G115 - BPF filter length is bounded by denied syscall table size
		Filter: &filter[0],
	}, nil
}

func bpfStmt(code uint16, k uint32) unix.SockFilter {
	return unix.SockFilter{Code: code, K: k}
}

func bpfJump(code uint16, k uint32, jt, jf uint8) unix.SockFilter {
	return unix.SockFilter{Code: code, Jt: jt, Jf: jf, K: k}
}

func seccompPolicy() (arch uint32, denied []int, err error) {
	arch, ok := seccompAuditArch()
	if !ok {
		return 0, nil, fmt.Errorf("seccomp: unsupported arch %s", runtime.GOARCH)
	}
	return arch, deniedSyscalls(), nil
}
