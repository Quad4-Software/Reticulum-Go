// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package sandbox

import (
	"fmt"
	"runtime"
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

func applySeccomp(cfg *common.ReticulumConfig) {
	if !seccompEnabled(cfg) {
		debug.Log(debug.DebugInfo, "Seccomp disabled by configuration")
		return
	}
	if err := installSeccompFilter(); err != nil {
		debug.Log(debug.DebugError, "Seccomp filter install failed (continuing)", "error", err)
		return
	}
	debug.Log(debug.DebugInfo, "Seccomp filter applied", "arch", runtime.GOARCH)
}

func installSeccompFilter() error {
	arch, denied, err := seccompPolicy()
	if err != nil {
		return err
	}

	filter := make([]unix.SockFilter, 0, 8+2*len(denied))
	// Validate architecture.
	filter = append(filter,
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, seccompDataArchOffset),
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, arch, 1, 0),
		bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_KILL_THREAD),
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, seccompDataNrOffset),
	)
	for _, nr := range denied {
		// if nr == denied: return EPERM else continue
		filter = append(filter,
			bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, uint32(nr), 0, 1), // #nosec G115 - syscall numbers are small positive constants
			bpfStmt(unix.BPF_RET|unix.BPF_K, seccompRetErrnoEPERM),
		)
	}
	filter = append(filter, bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ALLOW))

	prog := &unix.SockFprog{
		Len:    uint16(len(filter)), // #nosec G115 - BPF filter length is bounded by denied syscall table size
		Filter: &filter[0],
	}
	_, _, errno := unix.Syscall(unix.SYS_SECCOMP, uintptr(unix.SECCOMP_SET_MODE_FILTER), 0, uintptr(unsafe.Pointer(prog))) // #nosec G103 - required for SECCOMP_SET_MODE_FILTER
	if errno == unix.ENOSYS {
		// Fall back to prctl on kernels without seccomp syscall.
		if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(prog)), 0, 0); err != nil { // #nosec G103 - required for PR_SET_SECCOMP filter install
			return fmt.Errorf("prctl seccomp: %w", err)
		}
		return nil
	}
	if errno != 0 {
		return errno
	}
	return nil
}

func bpfStmt(code uint16, k uint32) unix.SockFilter {
	return unix.SockFilter{Code: code, K: k}
}

func bpfJump(code uint16, k uint32, jt, jf uint8) unix.SockFilter {
	return unix.SockFilter{Code: code, Jt: jt, Jf: jf, K: k}
}

func seccompPolicy() (arch uint32, denied []int, err error) {
	switch runtime.GOARCH {
	case "amd64":
		return unix.AUDIT_ARCH_X86_64, deniedSyscallsAMD64(), nil
	case "arm64":
		return unix.AUDIT_ARCH_AARCH64, deniedSyscallsARM64(), nil
	default:
		return 0, nil, fmt.Errorf("seccomp: unsupported arch %s", runtime.GOARCH)
	}
}

// deniedSyscallsAMD64 blocks high-risk operations while allowing normal Go and mesh I/O.
func deniedSyscallsAMD64() []int {
	return []int{
		unix.SYS_PTRACE,
		unix.SYS_MOUNT,
		unix.SYS_UMOUNT2,
		unix.SYS_REBOOT,
		unix.SYS_SWAPON,
		unix.SYS_SWAPOFF,
		unix.SYS_INIT_MODULE,
		unix.SYS_FINIT_MODULE,
		unix.SYS_DELETE_MODULE,
		unix.SYS_KEXEC_LOAD,
		unix.SYS_KEXEC_FILE_LOAD,
		unix.SYS_USERFAULTFD,
		unix.SYS_PERF_EVENT_OPEN,
		unix.SYS_BPF,
	}
}

func deniedSyscallsARM64() []int {
	return []int{
		unix.SYS_PTRACE,
		unix.SYS_MOUNT,
		unix.SYS_UMOUNT2,
		unix.SYS_REBOOT,
		unix.SYS_SWAPON,
		unix.SYS_SWAPOFF,
		unix.SYS_INIT_MODULE,
		unix.SYS_FINIT_MODULE,
		unix.SYS_DELETE_MODULE,
		unix.SYS_KEXEC_LOAD,
		unix.SYS_KEXEC_FILE_LOAD,
		unix.SYS_USERFAULTFD,
		unix.SYS_PERF_EVENT_OPEN,
		unix.SYS_BPF,
	}
}
