// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build windows || js

package main

// watchHeapDumpSignal is a no-op on platforms without SIGUSR1.
func watchHeapDumpSignal(dir string) {}
