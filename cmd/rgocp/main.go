// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Command rgocp is a thin wrapper around reticulum-go cp.
package main

import (
	"os"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/cli"
)

func main() {
	os.Exit(cli.RunCP(os.Args[1:]))
}
