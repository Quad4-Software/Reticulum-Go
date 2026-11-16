// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Command rgosnap is a thin wrapper around reticulum-go snapshot.
package main

import (
	"os"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/cli"
)

func main() {
	os.Exit(cli.RunSnapshot(os.Args[1:]))
}
