// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux && riscv64

package sandbox

import "golang.org/x/sys/unix"

func seccompAuditArch() (uint32, bool) {
	return unix.AUDIT_ARCH_RISCV64, true
}

// deniedSyscalls blocks high-risk operations while allowing normal Go and mesh I/O.
func deniedSyscalls() []int {
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
