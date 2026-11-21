//go:build !linux

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

func restrictLandlock(policy) error {
	return nil
}

func landlockAvailable() bool {
	return false
}
