//go:build !cgo || !(linux || darwin || windows || android || freebsd || openbsd || netbsd || dragonfly)

// SPDX-License-Identifier: LicenseRef-Reticulum
//
//revive:disable:var-naming
package io

func Open(_ Options) (Device, error) {
	return NewNullDevice(), nil
}

func Backend() string { return "null" }
