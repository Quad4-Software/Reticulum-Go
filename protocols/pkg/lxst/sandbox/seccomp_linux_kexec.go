//go:build linux && amd64

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

import "golang.org/x/sys/unix"

func kexecFileDenies() []int {
	return []int{unix.SYS_KEXEC_FILE_LOAD}
}
