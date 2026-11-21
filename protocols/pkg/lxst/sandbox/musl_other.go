//go:build !musl

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

func muslTagged() bool { return false }
