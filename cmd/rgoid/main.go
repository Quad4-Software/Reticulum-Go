// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Command rgoid is a thin wrapper around reticulum-go id.
package main

import (
	"os"

	"github.com/Quad4-Software/Reticulum-Go/pkg/cli"
)

func main() {
	os.Exit(cli.RunID(os.Args[1:]))
}
