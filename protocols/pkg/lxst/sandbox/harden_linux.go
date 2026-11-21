//go:build linux

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

import "golang.org/x/sys/unix"

func hardenProcess() {
	unix.Umask(0o077)
	_ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
	_ = unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0})
}
