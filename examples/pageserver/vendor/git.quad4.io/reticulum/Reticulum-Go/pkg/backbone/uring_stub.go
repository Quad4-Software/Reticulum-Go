// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !linux

package backbone

func UringProbeAllowed() bool { return false }
