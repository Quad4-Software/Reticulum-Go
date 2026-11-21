// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd

package backbone

func setNonblockFD(int) error { return nil }
