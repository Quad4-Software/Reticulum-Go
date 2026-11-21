//go:build !cgo || !(linux || darwin || windows || android || freebsd || openbsd || netbsd || dragonfly)

// SPDX-License-Identifier: LicenseRef-Reticulum
//
//revive:disable:var-naming
package io

type DeviceInfo struct {
	Name    string
	Capture bool
}

func ListDevices() ([]DeviceInfo, error) {
	return nil, nil
}
