// SPDX-License-Identifier: LicenseRef-Reticulum
package io

const (
	RoleCapture  = 0
	RolePlayback = 1
	RoleDuplex   = 2
)

// Options selects capture/playback role and optional device names.
// Names are case-insensitive substrings of miniaudio device names.
type Options struct {
	Role       int
	Speaker    string
	Microphone string
}

func NewDevice(duplex bool) (Device, error) {
	if duplex {
		return Open(Options{Role: RoleDuplex})
	}
	return Open(Options{Role: RoleCapture})
}
