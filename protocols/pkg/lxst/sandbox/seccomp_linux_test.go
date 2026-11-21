//go:build linux

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

import (
	"slices"
	"testing"

	"golang.org/x/sys/unix"
)

func TestFilterDeniesPtrace(t *testing.T) {
	if auditArch() == 0 {
		t.Skip("unknown arch")
	}
	found := slices.Contains(deniedSyscalls(), unix.SYS_PTRACE)
	if !found {
		t.Fatal("ptrace missing from deny list")
	}
	for _, nr := range kexecFileDenies() {
		if nr == 0 {
			t.Fatal("zero kexec deny")
		}
	}
	filters := seccompFilter(auditArch(), deniedSyscalls())
	if len(filters) < 5 {
		t.Fatalf("short filter %d", len(filters))
	}
	if filters[0].K != 4 {
		t.Fatal("filter must load arch from seccomp_data")
	}
}
