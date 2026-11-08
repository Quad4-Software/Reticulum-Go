// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !js && !wasip1 && !tinygo && ((darwin && (amd64 || arm64)) || (freebsd && (386 || amd64 || arm || arm64)) || (illumos && amd64) || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || s390x)) || (netbsd && amd64) || (openbsd && (amd64 || arm64)) || windows)

package sqlitestore

import "time"

func float64Now() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}
