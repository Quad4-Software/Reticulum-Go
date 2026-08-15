// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Command rgoprobe is a thin wrapper around reticulum-go probe.
package main

import (
	"os"

	"quad4/reticulum-go/pkg/cli"
)

func main() {
	os.Exit(cli.RunProbe(os.Args[1:]))
}
