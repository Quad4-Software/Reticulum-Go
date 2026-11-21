//go:build linux

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

var seccompInstalled bool

func restrictSeccomp() error {
	arch := auditArch()
	if arch == 0 {
		return nil
	}
	filters := seccompFilter(arch, deniedSyscalls())
	if err := loadSeccomp(filters); err != nil {
		if err == unix.ENOSYS || err == unix.EPERM || err == unix.EINVAL {
			return nil
		}
		return err
	}
	seccompInstalled = true
	return nil
}

func seccompAvailable() bool {
	return seccompInstalled
}

func auditArch() uint32 {
	switch runtime.GOARCH {
	case "amd64":
		return unix.AUDIT_ARCH_X86_64
	case "arm64":
		return unix.AUDIT_ARCH_AARCH64
	case "386":
		return unix.AUDIT_ARCH_I386
	case "arm":
		return unix.AUDIT_ARCH_ARM
	case "ppc64le":
		return unix.AUDIT_ARCH_PPC64LE
	case "ppc64":
		return unix.AUDIT_ARCH_PPC64
	default:
		return 0
	}
}

func deniedSyscalls() []int {
	out := []int{
		unix.SYS_PTRACE,
		unix.SYS_PROCESS_VM_READV,
		unix.SYS_PROCESS_VM_WRITEV,
		unix.SYS_KEXEC_LOAD,
		unix.SYS_INIT_MODULE,
		unix.SYS_FINIT_MODULE,
		unix.SYS_DELETE_MODULE,
		unix.SYS_MOUNT,
		unix.SYS_UMOUNT2,
		unix.SYS_PIVOT_ROOT,
		unix.SYS_SWAPON,
		unix.SYS_SWAPOFF,
		unix.SYS_REBOOT,
		unix.SYS_ACCT,
		unix.SYS_SETNS,
		unix.SYS_UNSHARE,
		unix.SYS_BPF,
		unix.SYS_PERF_EVENT_OPEN,
		unix.SYS_USERFAULTFD,
		unix.SYS_SYSLOG,
		unix.SYS_KEYCTL,
		unix.SYS_ADD_KEY,
		unix.SYS_REQUEST_KEY,
		unix.SYS_PERSONALITY,
		unix.SYS_KCMP,
		unix.SYS_OPEN_BY_HANDLE_AT,
		unix.SYS_NAME_TO_HANDLE_AT,
		unix.SYS_FANOTIFY_INIT,
		unix.SYS_MOVE_MOUNT,
		unix.SYS_FSOPEN,
		unix.SYS_FSCONFIG,
		unix.SYS_FSMOUNT,
		unix.SYS_FSPICK,
		unix.SYS_OPEN_TREE,
		unix.SYS_MOUNT_SETATTR,
	}
	out = append(out, extraDenies()...)
	out = append(out, kexecFileDenies()...)
	return out
}

func seccompFilter(arch uint32, deny []int) []unix.SockFilter {
	retEPERM := unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)
	filters := []unix.SockFilter{
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 4),
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, arch, 1, 0),
		bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_KILL_THREAD),
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 0),
	}
	for _, nr := range deny {
		if nr < 0 {
			continue
		}
		k := uint32(nr) // #nosec G115 -- syscall numbers are small positive ABI constants
		filters = append(filters,
			bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, k, 0, 1),
			bpfStmt(unix.BPF_RET|unix.BPF_K, retEPERM),
		)
	}
	filters = append(filters, bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ALLOW))
	return filters
}

func loadSeccomp(filters []unix.SockFilter) error {
	if len(filters) == 0 || len(filters) > 0xffff {
		return fmt.Errorf("seccomp filter length %d", len(filters))
	}
	prog := unix.SockFprog{
		Len:    uint16(len(filters)), // #nosec G115 -- length checked above
		Filter: &filters[0],
	}
	_, _, errno := unix.Syscall(
		unix.SYS_SECCOMP,
		uintptr(unix.SECCOMP_SET_MODE_FILTER),
		uintptr(unix.SECCOMP_FILTER_FLAG_TSYNC),
		uintptr(unsafe.Pointer(&prog)), // #nosec G103 -- sock_fprog needs the filter pointer
	)
	runtime.KeepAlive(filters)
	if errno == unix.EINVAL {
		_, _, errno = unix.Syscall(
			unix.SYS_SECCOMP,
			uintptr(unix.SECCOMP_SET_MODE_FILTER),
			0,
			uintptr(unsafe.Pointer(&prog)), // #nosec G103 -- sock_fprog needs the filter pointer
		)
		runtime.KeepAlive(filters)
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
