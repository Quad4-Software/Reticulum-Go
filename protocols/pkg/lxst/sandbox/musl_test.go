//go:build musl

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

import "testing"

func TestMuslTagApplied(t *testing.T) {
	if !muslTagged() {
		t.Fatal("musl tag not applied")
	}
}
