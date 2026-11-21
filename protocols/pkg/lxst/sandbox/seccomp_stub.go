//go:build !linux

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

func restrictSeccomp() error {
	return nil
}

func seccompAvailable() bool {
	return false
}
