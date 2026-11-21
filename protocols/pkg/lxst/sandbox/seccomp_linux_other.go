//go:build linux && !amd64 && !386

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

func extraDenies() []int {
	return nil
}
