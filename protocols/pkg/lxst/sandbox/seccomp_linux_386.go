//go:build linux && 386

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

import "golang.org/x/sys/unix"

func extraDenies() []int {
	return []int{unix.SYS_IOPL, unix.SYS_IOPERM}
}
