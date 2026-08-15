// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build (linux && !amd64 && !arm64 && !386 && !arm && !riscv64 && !ppc64 && !ppc64le) && !tinygo

package sandbox

func seccompAuditArch() (uint32, bool) {
	return 0, false
}

func deniedSyscalls() []int {
	return nil
}
