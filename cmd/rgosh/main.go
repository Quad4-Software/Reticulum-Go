// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Command rgosh is a thin wrapper around reticulum-go sh.
package main

import (
	"os"

	"quad4/reticulum-go/pkg/cli"
)

func main() {
	os.Exit(cli.RunSH(os.Args[1:]))
}
