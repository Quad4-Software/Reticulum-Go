// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Command rgodump is a thin wrapper around reticulum-go dump.
package main

import (
	"os"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/cli"
)

func main() {
	os.Exit(cli.RunDump(os.Args[1:]))
}
