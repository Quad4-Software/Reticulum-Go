// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Command rgoid is a thin wrapper around reticulum-go id.
package main

import (
	"os"

	"quad4/reticulum-go/pkg/cli"
)

func main() {
	os.Exit(cli.RunID(os.Args[1:]))
}
